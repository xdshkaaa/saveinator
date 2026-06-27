package queue

import (
	"encoding/json"
	"testing"
)

func TestTaskUserID_downloadPayload(t *testing.T) {
	body, err := json.Marshal(DownloadPayload{
		UserID: 339193247,
		URL:    "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	uid, ok := TaskUserID(TypeDownload, body)
	if !ok || uid != 339193247 {
		t.Fatalf("TaskUserID() = (%d, %v), want (339193247, true)", uid, ok)
	}
}

func TestTaskUserID_musicPayload(t *testing.T) {
	body, err := json.Marshal(MusicPayload{
		UserID: 42,
		Platform: "spotify",
	})
	if err != nil {
		t.Fatal(err)
	}

	uid, ok := TaskUserID(TypeSpotify, body)
	if !ok || uid != 42 {
		t.Fatalf("TaskUserID() = (%d, %v), want (42, true)", uid, ok)
	}
}

func TestTaskUserID_ignoresBroadcast(t *testing.T) {
	body, err := json.Marshal(BroadcastPayload{BroadcastID: 1})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := TaskUserID(TypeBroadcast, body); ok {
		t.Fatal("broadcast task should not match user filter")
	}
}

func TestTaskLockRef(t *testing.T) {
	body, err := json.Marshal(DownloadPayload{
		UserID:    1,
		LockScene: "tiktok",
		LockToken: "abc-123",
	})
	if err != nil {
		t.Fatal(err)
	}

	ref, ok := TaskLockRef(TypeTikTok, body)
	if !ok {
		t.Fatal("expected lock ref")
	}
	if ref.Scene != "tiktok" || ref.Token != "abc-123" {
		t.Fatalf("TaskLockRef() = %+v", ref)
	}
}

func TestTaskLockRef_emptyLock(t *testing.T) {
	body, err := json.Marshal(DownloadPayload{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := TaskLockRef(TypeDownload, body); ok {
		t.Fatal("expected no lock ref without token")
	}
}
