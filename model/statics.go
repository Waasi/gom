package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (b *BaseModel[T, PT]) Create(ctx context.Context, doc PT) (PT, error) {
	err := doc.Validate()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	doc.SetCreatedAt(now)
	doc.SetUpdatedAt(now)

	result, err := b.collection.InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.SetID(result.InsertedID.(primitive.ObjectID))
	doc.Bind(b.collection)
	return doc, nil
}

func (b *BaseModel[T, PT]) Find(ctx context.Context, filter bson.M, opts ...FindOptions) (*FindResult[T], error) {
	page := int64(1)
	limit := int64(25)

	if len(opts) > 0 {
		if opts[0].Page > 0 {
			page = opts[0].Page
		}
		if opts[0].Limit > 0 {
			limit = opts[0].Limit
		}
	}

	total, err := b.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	skip := (page - 1) * limit
	findOpts := options.Find().SetSkip(skip).SetLimit(limit)

	cursor, err := b.collection.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}

	results := make([]T, 0)
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	for i := range results {
		PT(&results[i]).Bind(b.collection)
	}

	totalPages := total / limit
	if total%limit != 0 {
		totalPages++
	}

	return &FindResult[T]{
		Data:       results,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func (b *BaseModel[T, PT]) FindOne(ctx context.Context, filter bson.M) (*T, error) {
	var result T
	err := b.collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}

	PT(&result).Bind(b.collection)
	return &result, nil
}

func (b *BaseModel[T, PT]) FindByID(ctx context.Context, id primitive.ObjectID) (*T, error) {
	var result T
	err := b.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		return nil, err
	}

	PT(&result).Bind(b.collection)
	return &result, nil
}

func (b *BaseModel[T, PT]) Count(ctx context.Context, filter bson.M) (int64, error) {
	return b.collection.CountDocuments(ctx, filter)
}

func (b *BaseModel[T, PT]) Stream(ctx context.Context, pipeline mongo.Pipeline, opts ...*options.ChangeStreamOptions) (*mongo.ChangeStream, error) {
	return b.collection.Watch(ctx, pipeline, opts...)
}

func (b *BaseModel[T, PT]) Aggregate(ctx context.Context, pipeline mongo.Pipeline) ([]bson.M, error) {
	cursor, err := b.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

func (b *BaseModel[T, PT]) UpdateByID(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	if update["$set"] != nil {
		update["$set"].(bson.M)["updatedAt"] = time.Now()
	} else {
		update["$set"] = bson.M{"updatedAt": time.Now()}
	}
	_, err := b.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

func (b *BaseModel[T, PT]) DeleteByID(ctx context.Context, id primitive.ObjectID) error {
	_, err := b.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (b *BaseModel[T, PT]) DeleteOne(ctx context.Context, filter bson.M) error {
	_, err := b.collection.DeleteOne(ctx, filter)
	return err
}

func (b *BaseModel[T, PT]) FindOneAndUpdate(ctx context.Context, filter bson.M, update bson.M, upsert bool) (*T, error) {
	if update["$set"] != nil {
		update["$set"].(bson.M)["updatedAt"] = time.Now()
	} else {
		update["$set"] = bson.M{"updatedAt": time.Now()}
	}
	if upsert {
		if update["$setOnInsert"] == nil {
			update["$setOnInsert"] = bson.M{}
		}
		update["$setOnInsert"].(bson.M)["createdAt"] = time.Now()
	}

	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.After).
		SetUpsert(upsert)

	var result T
	err := b.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
	if err != nil {
		return nil, err
	}

	PT(&result).Bind(b.collection)
	return &result, nil
}
