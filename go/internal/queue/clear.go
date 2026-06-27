package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const defaultQueueName = "default"

var userTaskTypes = map[string]struct{}{
	TypeDownload:      {},
	TypeTikTok:        {},
	TypePinterest:     {},
	TypeSpotify:       {},
	TypeSoundCloud:    {},
	TypeTikTokCarousel: {},
}

type LockRef struct {
	Scene string
	Token string
}

type ClearUserResult struct {
	DeletedPending  int
	CancelledActive int
}

func (r ClearUserResult) Any() bool {
	return r.DeletedPending > 0 || r.CancelledActive > 0
}

func NewInspector(redisURL string) (*asynq.Inspector, error) {
	opt, err := RedisOpt(redisURL)
	if err != nil {
		return nil, err
	}
	return asynq.NewInspector(opt), nil
}

func TaskUserID(taskType string, payload []byte) (int64, bool) {
	if _, ok := userTaskTypes[taskType]; !ok {
		return 0, false
	}
	switch taskType {
	case TypeSpotify, TypeSoundCloud:
		var p MusicPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return 0, false
		}
		return p.UserID, p.UserID != 0
	default:
		var p DownloadPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return 0, false
		}
		return p.UserID, p.UserID != 0
	}
}

func TaskLockRef(taskType string, payload []byte) (LockRef, bool) {
	if _, ok := userTaskTypes[taskType]; !ok {
		return LockRef{}, false
	}
	switch taskType {
	case TypeSpotify, TypeSoundCloud:
		var p MusicPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return LockRef{}, false
		}
		if p.LockScene == "" || p.LockToken == "" {
			return LockRef{}, false
		}
		return LockRef{Scene: p.LockScene, Token: p.LockToken}, true
	default:
		var p DownloadPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return LockRef{}, false
		}
		if p.LockScene == "" || p.LockToken == "" {
			return LockRef{}, false
		}
		return LockRef{Scene: p.LockScene, Token: p.LockToken}, true
	}
}

func ClearUserTasks(insp *asynq.Inspector, userID int64) (ClearUserResult, []LockRef, error) {
	var result ClearUserResult
	var lockRefs []LockRef

	active, err := insp.ListActiveTasks(defaultQueueName)
	if err != nil {
		return result, nil, err
	}
	for _, task := range active {
		uid, ok := TaskUserID(task.Type, task.Payload)
		if !ok || uid != userID {
			continue
		}
		if ref, ok := TaskLockRef(task.Type, task.Payload); ok {
			lockRefs = append(lockRefs, ref)
		}
		if err := insp.CancelProcessing(task.ID); err == nil {
			result.CancelledActive++
		}
	}

	for _, listFn := range []func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error){
		insp.ListPendingTasks,
		insp.ListScheduledTasks,
		insp.ListRetryTasks,
	} {
		tasks, err := listFn(defaultQueueName)
		if err != nil {
			return result, lockRefs, err
		}
		for _, task := range tasks {
			uid, ok := TaskUserID(task.Type, task.Payload)
			if !ok || uid != userID {
				continue
			}
			if err := insp.DeleteTask(defaultQueueName, task.ID); err == nil {
				result.DeletedPending++
			}
		}
	}

	return result, lockRefs, nil
}
