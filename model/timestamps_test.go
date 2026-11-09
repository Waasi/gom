package model

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestWithTimestamps(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name   string
		update bson.M
		upsert bool
		check  func(t *testing.T, got bson.M)
	}{
		{"nil update", nil, false, func(t *testing.T, got bson.M) {
			if got["$set"].(bson.M)["updatedAt"] != now {
				t.Errorf("got %v", got)
			}
		}},
		{"bson.M $set", bson.M{"$set": bson.M{"a": 1}}, false, func(t *testing.T, got bson.M) {
			s := got["$set"].(bson.M)
			if s["a"] != 1 || s["updatedAt"] != now {
				t.Errorf("got %v", s)
			}
		}},
		{"plain map $set", bson.M{"$set": map[string]any{"a": 1}}, false, func(t *testing.T, got bson.M) {
			if got["$set"].(map[string]any)["updatedAt"] != now {
				t.Errorf("got %v", got)
			}
		}},
		{"bson.D $set", bson.M{"$set": bson.D{{Key: "a", Value: 1}}}, false, func(t *testing.T, got bson.M) {
			d := got["$set"].(bson.D)
			if len(d) != 2 || d[1].Key != "updatedAt" || d[1].Value != now {
				t.Errorf("got %v", d)
			}
		}},
		{"explicit updatedAt kept", bson.M{"$set": bson.M{"updatedAt": earlier}}, false, func(t *testing.T, got bson.M) {
			if got["$set"].(bson.M)["updatedAt"] != earlier {
				t.Errorf("got %v", got)
			}
		}},
		{"upsert adds createdAt", bson.M{"$push": bson.M{"tags": "x"}}, true, func(t *testing.T, got bson.M) {
			if got["$setOnInsert"].(bson.M)["createdAt"] != now || got["$push"] == nil {
				t.Errorf("got %v", got)
			}
		}},
		{"upsert bson.D $setOnInsert", bson.M{"$setOnInsert": bson.D{{Key: "a", Value: 1}}}, true, func(t *testing.T, got bson.M) {
			if d := got["$setOnInsert"].(bson.D); len(d) != 2 || d[1].Key != "createdAt" {
				t.Errorf("got %v", d)
			}
		}},
		{"createdAt in $set is not duplicated", bson.M{"$set": bson.M{"createdAt": earlier}}, true, func(t *testing.T, got bson.M) {
			if _, ok := got["$setOnInsert"]; ok {
				t.Errorf("$setOnInsert also sets createdAt: %v", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := withTimestamps(tt.update, now, tt.upsert)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, got)
		})
	}
}

func TestWithTimestampsDoesNotModifyInput(t *testing.T) {
	set := bson.M{"a": 1}
	in := bson.M{"$set": set}
	if _, err := withTimestamps(in, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if len(in) != 1 || len(set) != 1 {
		t.Fatalf("input modified: %v", in)
	}
}

func TestWithTimestampsRejectsNonDocument(t *testing.T) {
	if _, err := withTimestamps(bson.M{"$set": "oops"}, time.Now(), false); err == nil {
		t.Fatal("want an error for a string $set")
	}
}
