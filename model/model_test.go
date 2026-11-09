package model_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/waasi/gom/internal/testmongo"
	"github.com/waasi/gom/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testmongo.Stop()
	os.Exit(code)
}

// widget is what `gom generate model widget name:string! count:int` produces.
type widget struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Name      string             `bson:"name" validate:"required"`
	Count     int                `bson:"count"`
	CreatedAt time.Time          `bson:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt,omitempty"`

	collection *mongo.Collection
}

func (w *widget) Validate() error             { return model.Validator.Struct(w) }
func (w *widget) GetID() primitive.ObjectID   { return w.ID }
func (w *widget) SetID(id primitive.ObjectID) { w.ID = id }
func (w *widget) Bind(c *mongo.Collection)    { w.collection = c }
func (w *widget) SetCreatedAt(t time.Time)    { w.CreatedAt = t }
func (w *widget) SetUpdatedAt(t time.Time)    { w.UpdatedAt = t }

func TestBaseModel(t *testing.T) {
	db := testmongo.DB(t)
	ctx := context.Background()
	m := model.New[widget](db.Collection("widgets"))

	var verr validator.ValidationErrors
	if _, err := m.Create(ctx, &widget{}); !errors.As(err, &verr) {
		t.Fatalf("Create without name: err = %v, want a validation error", err)
	}

	w, err := m.Create(ctx, &widget{Name: "a", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if w.ID.IsZero() || w.CreatedAt.IsZero() || w.collection == nil {
		t.Fatalf("Create did not set id/timestamps/collection: %+v", w)
	}
	for i := 0; i < 4; i++ {
		if _, err := m.Create(ctx, &widget{Name: "b", Count: i}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := m.FindByID(ctx, w.ID)
	if err != nil || got.Name != "a" || got.collection == nil {
		t.Fatalf("FindByID = %+v, %v", got, err)
	}

	page, err := m.Find(ctx, bson.M{"name": "b"}, model.FindOptions{Page: 2, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || page.TotalPages != 2 || len(page.Data) != 1 || page.Page != 2 {
		t.Fatalf("Find page 2 = total %d pages %d len %d page %d", page.Total, page.TotalPages, len(page.Data), page.Page)
	}

	before := got.UpdatedAt
	time.Sleep(5 * time.Millisecond)
	if err := m.UpdateByID(ctx, w.ID, bson.M{"$set": bson.M{"count": 9}}); err != nil {
		t.Fatal(err)
	}
	got, _ = m.FindByID(ctx, w.ID)
	if got.Count != 9 || !got.UpdatedAt.After(before) {
		t.Fatalf("UpdateByID: count %d, updatedAt %v (before %v)", got.Count, got.UpdatedAt, before)
	}

	// $set as bson.D used to panic on a type assertion.
	if err := m.UpdateByID(ctx, w.ID, bson.M{"$set": bson.D{{Key: "count", Value: 10}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = m.FindByID(ctx, w.ID)
	if got.Count != 10 {
		t.Fatalf("UpdateByID with bson.D: count = %d, want 10", got.Count)
	}

	up, err := m.FindOneAndUpdate(ctx, bson.M{"name": "c"}, bson.M{"$set": bson.M{"count": 3}}, true)
	if err != nil || up.Name != "c" || up.CreatedAt.IsZero() {
		t.Fatalf("FindOneAndUpdate upsert = %+v, %v", up, err)
	}

	if n, _ := m.Count(ctx, bson.M{}); n != 6 {
		t.Fatalf("Count = %d, want 6", n)
	}
	if err := m.DeleteByID(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FindByID(ctx, w.ID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("FindByID after delete: %v", err)
	}
}
