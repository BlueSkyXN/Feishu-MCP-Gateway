// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import "time"

// 仅在文件写入成功后替换正文和文件列表；失败时调用者仍持有完整结果。
func (s *Service) archiveResult(p Principal, r Result) (Result, string, error) {
	payload := encode(r)
	if len(payload) <= s.config.MaxInlineBytes && (r.Error == nil || r.Data == nil) {
		return r, "", nil
	}
	f, err := s.Store.PutFile(p, "result.json", "application/json", payload, r.InvocationID)
	if err != nil {
		return r, "", err
	}
	r.Data = nil
	r.Meta = map[string]any{"result_spilled": true, "artifact_count": len(r.Files)}
	r.Files = []File{f}
	return r, f.ID, nil
}

func (s *Service) resultDeliveryFailure(original Result, err error) Result {
	r := original
	if r.Status == "succeeded" {
		r.Status = "failed"
	}
	r.Data = nil
	r.Meta = map[string]any{
		"result_spilled": false, "result_retained": true,
		"execution_status": original.Status, "artifact_count": len(original.Files),
		"storage_error": asFault(err).Kind,
	}
	r.Error = fault("result_storage_failed", "完整结果已保留在调用记录；释放配额、调整文件限额或排除存储故障后，用 feishu_job_get 重取。写入操作请复用原幂等键，避免重复执行")
	// 已归档的文件继续可用；过长的文件清单保留在完整回执中。
	if len(encode(r)) > s.config.MaxInlineBytes {
		r.Files = nil
	}
	return r
}

// 查询或同键重试只恢复结果归档，不再次调用执行器。数据目录由单进程独占，
// 服务内串行恢复并重新读取回执，避免并发查询创建重复的 result.json。
func (s *Service) resultFor(p Principal, record Record) Result {
	if record.RetainedResult == nil {
		return record.Result
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if record.RetainedUntil <= time.Now().Unix() {
		if err := expireRetainedResults(s.Store.db); err != nil {
			return failed(err)
		}
	}
	current, err := s.Store.Record(p, record.ID)
	if err != nil {
		return failed(err)
	}
	op, ok := s.Catalog.Operation(current.OperationID)
	if !ok {
		return failed(fault("operation_removed", "原操作已不可用"))
	}
	if _, err := authorize(s.currentConfig(), p, op, current.Arguments); err != nil {
		return failed(err)
	}
	if current.RetainedResult == nil {
		return current.Result
	}
	r, createdFile, err := s.archiveResult(p, *current.RetainedResult)
	if err != nil {
		return current.Result
	}
	if err = s.Store.SaveRecoveredResult(p, current.ID, r); err != nil {
		// 新归档尚未成为回执入口；保留数据库中的完整结果供下次恢复。
		if createdFile != "" {
			_ = s.Store.DeleteFile(p, createdFile)
		}
		return Result{InvocationID: current.ID, OperationID: current.OperationID, Status: "unknown", Error: fault("receipt_storage_failed", "结果归档后未能更新回执，请重新查询原调用记录")}
	}
	return r
}
