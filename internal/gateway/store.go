package gateway

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	files *os.Root
	cfg   Config
	lock  *os.File
}
type Record struct {
	ID          string
	Principal   Principal
	OperationID string
	Arguments   map[string]any
	State       string
	Result      Result
	Async       bool
}

func OpenStore(c Config) (*Store, error) {
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(c.DataDir, ".gateway.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("数据目录已被另一个 Gateway 进程占用")
	}
	s, err := openStore(c)
	if err != nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
		return nil, err
	}
	s.lock = lock
	return s, nil
}

func openStore(c Config) (*Store, error) {
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, err
	}
	dir := filepath.Join(c.DataDir, "files")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	dbpath := filepath.Join(c.DataDir, "gateway.db")
	f, err := os.OpenFile(dbpath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		root.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, files: root, cfg: c}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		s.Close()
		return nil, err
	}
	if version > 1 {
		s.Close()
		return nil, fmt.Errorf("数据库版本 %d 高于当前程序支持的版本 1", version)
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS invocations(id TEXT PRIMARY KEY,owner TEXT NOT NULL,principal TEXT NOT NULL,operation TEXT NOT NULL,args TEXT NOT NULL,idem TEXT NOT NULL,hash TEXT NOT NULL,state TEXT NOT NULL,result TEXT NOT NULL,async INTEGER NOT NULL,created INTEGER NOT NULL,updated INTEGER NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS invocation_idem ON invocations(owner,operation,idem) WHERE idem<>'';
CREATE TABLE IF NOT EXISTS files(id TEXT PRIMARY KEY,owner TEXT NOT NULL,client TEXT NOT NULL,meta TEXT NOT NULL,size INTEGER NOT NULL,expires INTEGER NOT NULL,state TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS cursors(id TEXT PRIMARY KEY,owner TEXT NOT NULL,operation TEXT NOT NULL,query_hash TEXT NOT NULL,token TEXT NOT NULL,expires INTEGER NOT NULL);
PRAGMA user_version=1;`)
	if err != nil {
		s.Close()
		return nil, err
	}
	// 已经开始的外部请求无法仅凭本地状态判断是否完成。
	_, err = db.Exec(`UPDATE invocations SET state='unknown',result=json_set(result,'$.status','unknown','$.error',json(?)) WHERE state IN ('running','cancel_requested')`, string(encode(fault("process_restarted", "进程中断，执行结果需要核对"))))
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	s.files.Close()
	err := s.db.Close()
	if s.lock != nil {
		syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		s.lock.Close()
	}
	return err
}

func scanRecord(row interface{ Scan(...any) error }) (Record, error) {
	var r Record
	var p, a, b string
	err := row.Scan(&r.ID, &p, &r.OperationID, &a, &r.State, &b, &r.Async)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(p), &r.Principal); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(a), &r.Arguments); err != nil {
		return r, err
	}
	err = json.Unmarshal([]byte(b), &r.Result)
	return r, err
}

const recordColumns = `id,principal,operation,args,state,result,async`

func (s *Store) Reserve(p Principal, op Operation, args map[string]any, idem string, async bool) (Record, bool, error) {
	semantic := map[string]any{}
	for k, v := range args {
		if k != "idempotency_key" && k != "async" {
			semantic[k] = v
		}
	}
	hash := digest(encode(semantic))
	id := newID("inv_")
	state := "running"
	if async {
		state = "queued"
	}
	result := Result{InvocationID: id, OperationID: op.ID, Status: state}
	if async {
		result.JobID = id
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Record{}, false, err
	}
	defer tx.Rollback()
	if idem != "" {
		var oldHash string
		err = tx.QueryRow(`SELECT hash FROM invocations WHERE owner=? AND operation=? AND idem=?`, p.Owner(), op.ID, idem).Scan(&oldHash)
		if err == nil {
			if hash != oldHash {
				return Record{}, false, fault("idempotency_conflict", "同一幂等键已用于不同参数")
			}
			r, err := scanRecord(tx.QueryRow(`SELECT `+recordColumns+` FROM invocations WHERE owner=? AND operation=? AND idem=?`, p.Owner(), op.ID, idem))
			return r, true, err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Record{}, false, err
		}
	}
	now := time.Now().Unix()
	if async {
		var pending int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM invocations WHERE json_extract(principal,'$.client_id')=? AND state IN ('queued','running','cancel_requested')`, p.ClientID).Scan(&pending); err != nil {
			return Record{}, false, err
		}
		if pending >= s.cfg.MaxPendingJobs {
			return Record{}, false, fault("job_queue_limit", "接入后台队列已达上限，请等待已有操作完成")
		}
	}
	_, err = tx.Exec(`INSERT INTO invocations VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, p.Owner(), string(encode(p)), op.ID, string(encode(args)), idem, hash, state, string(encode(result)), async, now, now)
	if err != nil {
		return Record{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Record{}, false, err
	}
	return Record{ID: id, Principal: p, OperationID: op.ID, Arguments: args, State: state, Result: result, Async: async}, false, nil
}
func (s *Store) Finish(id string, r Result) error {
	_, err := s.db.Exec(`UPDATE invocations SET state=?,result=?,updated=? WHERE id=? AND state IN ('running','cancel_requested')`, r.Status, string(encode(r)), time.Now().Unix(), id)
	return err
}
func (s *Store) Record(p Principal, id string) (Record, error) {
	r, err := scanRecord(s.db.QueryRow(`SELECT `+recordColumns+` FROM invocations WHERE id=? AND owner=?`, id, p.Owner()))
	if errors.Is(err, sql.ErrNoRows) {
		return r, fault("not_found", "执行记录不存在或不在当前主体范围内")
	}
	return r, err
}
func (s *Store) Claim() (Record, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	r, err := scanRecord(tx.QueryRow(`SELECT ` + recordColumns + ` FROM invocations WHERE state='queued' ORDER BY created,id LIMIT 1`))
	if err != nil {
		return r, err
	}
	r.State = "running"
	r.Result.Status = "running"
	_, err = tx.Exec(`UPDATE invocations SET state='running',result=?,updated=? WHERE id=? AND state='queued'`, string(encode(r.Result)), time.Now().Unix(), r.ID)
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func (s *Store) Cancel(p Principal, id string) (Result, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	r, err := scanRecord(tx.QueryRow(`SELECT `+recordColumns+` FROM invocations WHERE id=? AND owner=?`, id, p.Owner()))
	if err != nil {
		return Result{}, fault("not_found", "执行记录不存在或不在当前主体范围内")
	}
	switch r.State {
	case "queued":
		r.Result.Status = "cancelled"
	case "running":
		r.Result.Status = "cancel_requested"
	default:
		return r.Result, nil
	}
	_, err = tx.Exec(`UPDATE invocations SET state=?,result=?,updated=? WHERE id=?`, r.Result.Status, string(encode(r.Result)), time.Now().Unix(), id)
	if err != nil {
		return Result{}, err
	}
	return r.Result, tx.Commit()
}

func (s *Store) PutFile(p Principal, name, mime string, b []byte, source ...string) (File, error) {
	if int64(len(b)) > s.cfg.MaxFileBytes {
		return File{}, fault("file_too_large", "文件超过单文件容量限制")
	}
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return File{}, fault("invalid_filename", "名称必须是没有目录和换行的短文件名")
	}
	f := File{ID: newID("f_"), Name: name, MIME: mime, Size: int64(len(b)), SHA256: digest(b), Expires: time.Now().Unix() + s.cfg.FileTTL}
	f.URI = "feishu-file://files/" + f.ID
	if len(source) > 0 {
		f.SourceInvocation = source[0]
	}
	tx, err := s.db.Begin()
	if err != nil {
		return f, err
	}
	defer tx.Rollback()
	var used int64
	if err = tx.QueryRow(`SELECT COALESCE(SUM(size),0) FROM files WHERE client=? AND state='ready' AND expires>?`, p.ClientID, time.Now().Unix()).Scan(&used); err != nil {
		return f, err
	}
	if used+f.Size > s.cfg.MaxStorageBytes {
		return f, fault("storage_quota", "接入文件容量不足，请清理文件")
	}
	out, err := s.files.OpenFile(f.ID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return f, err
	}
	_, writeErr := out.Write(b)
	syncErr := out.Sync()
	closeErr := out.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		s.files.Remove(f.ID)
		return f, errors.Join(writeErr, syncErr, closeErr)
	}
	_, err = tx.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, f.ID, p.Owner(), p.ClientID, string(encode(f)), f.Size, f.Expires, "ready")
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.files.Remove(f.ID)
	}
	return f, err
}

func (s *Store) File(p Principal, id string) (File, error) {
	var raw, state string
	var f File
	err := s.db.QueryRow(`SELECT meta,state FROM files WHERE id=? AND owner=?`, id, p.Owner()).Scan(&raw, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return f, fault("not_found", "文件不存在或不在当前主体范围内")
	}
	if err != nil {
		return f, err
	}
	if err = json.Unmarshal([]byte(raw), &f); err != nil {
		return f, err
	}
	if state != "ready" || f.Expires <= time.Now().Unix() {
		return f, fault("file_expired", "文件已过期或删除")
	}
	return f, nil
}
func (s *Store) ReadFile(p Principal, id string, offset int64, limit int) (File, []byte, error) {
	f, err := s.File(p, id)
	if err != nil {
		return f, nil, err
	}
	if offset < 0 || offset > f.Size || limit < 1 || limit > 256<<10 {
		return f, nil, fault("invalid_range", "读取范围无效或超过 256 KiB")
	}
	in, err := s.files.Open(id)
	if err != nil {
		return f, nil, err
	}
	defer in.Close()
	if _, err = in.Seek(offset, io.SeekStart); err != nil {
		return f, nil, err
	}
	b, err := io.ReadAll(io.LimitReader(in, int64(limit)))
	return f, b, err
}
func (s *Store) DeleteFile(p Principal, id string) error {
	_, err := s.File(p, id)
	if err != nil {
		return err
	}
	if err = s.files.Remove(id); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.db.Exec(`UPDATE files SET state='deleted' WHERE id=? AND owner=?`, id, p.Owner())
	return err
}
func (s *Store) Collect() error {
	rows, err := s.db.Query(`SELECT id FROM files WHERE state='ready' AND expires<=?`, time.Now().Unix())
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.files.Remove(id); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err = s.db.Exec(`UPDATE files SET state='expired' WHERE id=?`, id); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`DELETE FROM cursors WHERE expires<=?`, time.Now().Unix())
	return err
}

func queryHash(args map[string]any) string {
	m := map[string]any{}
	for k, v := range args {
		if k != "async" && k != "idempotency_key" {
			m[k] = v
		}
	}
	p := map[string]any{}
	for k, v := range object(m["params"]) {
		if k != "page_token" {
			p[k] = v
		}
	}
	m["params"] = p
	return digest(encode(m))
}
func (s *Store) WrapCursor(p Principal, op string, args map[string]any, token string) (string, error) {
	id := newID("cur_")
	_, err := s.db.Exec(`INSERT INTO cursors VALUES(?,?,?,?,?,?)`, id, p.Owner(), op, queryHash(args), token, time.Now().Unix()+3600)
	return id, err
}
func (s *Store) UnwrapCursor(p Principal, op string, args map[string]any) error {
	params := object(args["params"])
	id, _ := params["page_token"].(string)
	if id == "" {
		return nil
	}
	var token string
	err := s.db.QueryRow(`SELECT token FROM cursors WHERE id=? AND owner=? AND operation=? AND query_hash=? AND expires>?`, id, p.Owner(), op, queryHash(args), time.Now().Unix()).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return fault("invalid_cursor", "分页标识已过期，或不属于当前主体和查询")
	}
	if err != nil {
		return fmt.Errorf("cursor: %w", err)
	}
	params["page_token"] = token
	return nil
}
