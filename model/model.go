package model

import "go.mongodb.org/mongo-driver/mongo"

func New[T any, PT Ptr[T]](collection *mongo.Collection) BaseModel[T, PT] {
	return BaseModel[T, PT]{
		collection: collection,
	}
}
