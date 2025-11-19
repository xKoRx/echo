package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	bolt "go.etcd.io/bbolt"
)

const ackBucketName = "acks"

type AckLedger struct {
	db *bolt.DB
}

type AckRecord struct {
	CommandID       string      `json:"command_id"`
	TargetAccountID string      `json:"target_account_id"`
	Payload         []byte      `json:"payload"`
	PayloadType     string      `json:"payload_type"`
	Stage           pb.AckStage `json:"stage"`
	Attempt         uint32      `json:"attempt"`
	NextRetryAt     int64       `json:"next_retry_at"`
	UpdatedAt       int64       `json:"updated_at"`
	LastError       string      `json:"last_error"`
}

func OpenAckLedger(path string) (*AckLedger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir ledger path: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open ledger: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(ackBucketName))
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &AckLedger{db: db}, nil
}

func (l *AckLedger) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

func (l *AckLedger) Put(record *AckRecord) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ackBucketName))
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return b.Put([]byte(record.CommandID), data)
	})
}

func (l *AckLedger) Get(commandID string) (*AckRecord, error) {
	var rec *AckRecord
	err := l.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(ackBucketName)).Get([]byte(commandID))
		if len(data) == 0 {
			return nil
		}
		var r AckRecord
		if err := json.Unmarshal(data, &r); err != nil {
			return err
		}
		rec = &r
		return nil
	})
	return rec, err
}

func (l *AckLedger) UpdateStage(commandID string, stage pb.AckStage, attempt uint32, nextRetry time.Time, lastError string) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ackBucketName))
		key := []byte(commandID)
		data := b.Get(key)
		if len(data) == 0 {
			return nil
		}
		var rec AckRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Stage = stage
		rec.Attempt = attempt
		rec.NextRetryAt = nextRetry.UnixMilli()
		rec.UpdatedAt = time.Now().UnixMilli()
		rec.LastError = lastError
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(key, updated)
	})
}

func (l *AckLedger) Delete(commandID string) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(ackBucketName)).Delete([]byte(commandID))
	})
}

func (l *AckLedger) ListDue(before time.Time, limit int) ([]*AckRecord, error) {
	results := make([]*AckRecord, 0, limit)
	err := l.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(ackBucketName)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if len(v) == 0 {
				continue
			}
			var rec AckRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				continue
			}
			if rec.NextRetryAt == 0 || time.UnixMilli(rec.NextRetryAt).After(before) {
				continue
			}
			results = append(results, &rec)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
		return nil
	})
	return results, err
}
