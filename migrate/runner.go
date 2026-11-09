package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Record is one row of schema_migrations.
type Record struct {
	Version   int64     `bson:"_id"`
	Name      string    `bson:"name"`
	AppliedAt time.Time `bson:"appliedAt"`
}

// StatusRow pairs a known migration with whether it has been applied.
type StatusRow struct {
	Migration
	Applied   bool
	AppliedAt time.Time
}

// Runner applies and rolls back migrations against DB.
type Runner struct {
	DB         *mongo.Database
	Migrations []Migration // defaults to Registered()
	Log        io.Writer   // progress output; nil discards it
}

func (r *Runner) migrations() []Migration {
	if r.Migrations != nil {
		ms := append([]Migration(nil), r.Migrations...)
		sortMigrations(ms)
		return ms
	}
	return Registered()
}

func (r *Runner) logf(format string, args ...any) {
	if r.Log != nil {
		fmt.Fprintf(r.Log, format+"\n", args...)
	}
}

func (r *Runner) col() *mongo.Collection { return r.DB.Collection(Collection) }

func (r *Runner) applied(ctx context.Context) (map[int64]Record, error) {
	cur, err := r.col().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var recs []Record
	if err := cur.All(ctx, &recs); err != nil {
		return nil, err
	}
	out := make(map[int64]Record, len(recs))
	for _, rec := range recs {
		out[rec.Version] = rec
	}
	return out, nil
}

// Up applies every pending migration in version order and returns how many
// ran. It stops at the first failure; a failed migration is not recorded, so
// the next run retries it.
func (r *Runner) Up(ctx context.Context) (int, error) {
	done, err := r.applied(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range r.migrations() {
		if _, ok := done[m.Version]; ok {
			continue
		}
		r.logf("== %d %s: migrating", m.Version, m.Name)
		start := time.Now()
		if err := m.Up(ctx, r.DB); err != nil {
			return n, fmt.Errorf("migration %d (%s) up: %w", m.Version, m.Name, err)
		}
		rec := Record{Version: m.Version, Name: m.Name, AppliedAt: time.Now().UTC()}
		if _, err := r.col().InsertOne(ctx, rec); err != nil {
			return n, fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		r.logf("== %d %s: migrated (%s)", m.Version, m.Name, time.Since(start).Round(time.Millisecond))
		n++
	}
	return n, nil
}

// ErrIrreversible is returned when rolling back a migration with no Down.
var ErrIrreversible = errors.New("migration has no Down")

// Down rolls back the most recently applied steps migrations, newest first,
// and returns how many were rolled back.
func (r *Runner) Down(ctx context.Context, steps int) (int, error) {
	if steps <= 0 {
		return 0, nil
	}
	known := map[int64]Migration{}
	for _, m := range r.migrations() {
		known[m.Version] = m
	}
	cur, err := r.col().Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(steps)))
	if err != nil {
		return 0, err
	}
	var recs []Record
	if err := cur.All(ctx, &recs); err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range recs {
		m, ok := known[rec.Version]
		if !ok {
			return n, fmt.Errorf("migration %d (%s) is applied but its code is not compiled in", rec.Version, rec.Name)
		}
		if m.Down == nil {
			return n, fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, ErrIrreversible)
		}
		r.logf("== %d %s: reverting", m.Version, m.Name)
		start := time.Now()
		if err := m.Down(ctx, r.DB); err != nil {
			return n, fmt.Errorf("migration %d (%s) down: %w", m.Version, m.Name, err)
		}
		if _, err := r.col().DeleteOne(ctx, bson.M{"_id": m.Version}); err != nil {
			return n, fmt.Errorf("unrecord migration %d: %w", m.Version, err)
		}
		r.logf("== %d %s: reverted (%s)", m.Version, m.Name, time.Since(start).Round(time.Millisecond))
		n++
	}
	return n, nil
}

// Status lists every known migration with its applied state, plus any applied
// version whose code is missing (Name taken from the record, Up nil).
func (r *Runner) Status(ctx context.Context) ([]StatusRow, error) {
	done, err := r.applied(ctx)
	if err != nil {
		return nil, err
	}
	var rows []StatusRow
	seen := map[int64]bool{}
	for _, m := range r.migrations() {
		rec, ok := done[m.Version]
		rows = append(rows, StatusRow{Migration: m, Applied: ok, AppliedAt: rec.AppliedAt})
		seen[m.Version] = true
	}
	for v, rec := range done {
		if !seen[v] {
			rows = append(rows, StatusRow{Migration: Migration{Version: v, Name: rec.Name}, Applied: true, AppliedAt: rec.AppliedAt})
		}
	}
	sortRows(rows)
	return rows, nil
}
