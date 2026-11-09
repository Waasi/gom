package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type FindOptions struct {
	Page  int64
	Limit int64
}

type FindResult[T any] struct {
	Data       []T   `json:"data"`
	Total      int64 `json:"total"`
	Page       int64 `json:"page"`
	Limit      int64 `json:"limit"`
	TotalPages int64 `json:"totalPages"`
}

type Ptr[T any] interface {
	IModel
	*T
}

type BaseModel[T any, PT Ptr[T]] struct {
	collection *mongo.Collection
}

type IModel interface {
	Validate() error
	GetID() primitive.ObjectID
	SetID(primitive.ObjectID)
	Bind(*mongo.Collection)
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
}
