package model

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// withTimestamps returns a copy of update with updatedAt added to $set, and,
// when upsert, createdAt added to $setOnInsert. A timestamp the caller set
// explicitly is kept, which lets migrations preserve original times; and
// createdAt is left out of $setOnInsert when $set already writes it, since
// MongoDB rejects an update that sets one path twice.
func withTimestamps(update bson.M, now time.Time, upsert bool) (bson.M, error) {
	out := make(bson.M, len(update)+2)
	for k, v := range update {
		out[k] = v
	}
	set, err := addField("$set", out["$set"], "updatedAt", now)
	if err != nil {
		return nil, err
	}
	out["$set"] = set
	if upsert && !hasField(set, "createdAt") {
		soi, err := addField("$setOnInsert", out["$setOnInsert"], "createdAt", now)
		if err != nil {
			return nil, err
		}
		out["$setOnInsert"] = soi
	}
	return out, nil
}

// addField returns a copy of doc with key set to val, unless key is already
// present.
func addField(op string, doc any, key string, val any) (any, error) {
	switch d := doc.(type) {
	case nil:
		return bson.M{key: val}, nil
	case bson.M:
		return addToMap(d, key, val), nil
	case map[string]any:
		return addToMap(d, key, val), nil
	case bson.D:
		c := append(bson.D(nil), d...)
		if !hasField(c, key) {
			c = append(c, bson.E{Key: key, Value: val})
		}
		return c, nil
	}
	return nil, fmt.Errorf("model: %s must be a document (bson.M, map[string]any or bson.D), got %T", op, doc)
}

func addToMap[M ~map[string]any](m M, key string, val any) M {
	c := make(M, len(m)+1)
	for k, v := range m {
		c[k] = v
	}
	if _, ok := c[key]; !ok {
		c[key] = val
	}
	return c
}

func hasField(doc any, key string) bool {
	switch d := doc.(type) {
	case bson.M:
		_, ok := d[key]
		return ok
	case map[string]any:
		_, ok := d[key]
		return ok
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return true
			}
		}
	}
	return false
}
