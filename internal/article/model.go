package article

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type Article struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	ExternalID   int64              `bson:"externalId"`
	Type         string             `bson:"type"`
	Title        string             `bson:"title"`
	Description  string             `bson:"description"`
	Date         time.Time          `bson:"date"`
	Location     string             `bson:"location"`
	Language     string             `bson:"language"`
	CanonicalURL string             `bson:"canonicalUrl"`
	LastModified time.Time          `bson:"lastModified"`
	Body         string             `bson:"body"`
	Summary      string             `bson:"summary"`
	LeadMedia    LeadMedia          `bson:"leadMedia"`
	CreatedAt    time.Time          `bson:"createdAt"`
	ModifiedAt   time.Time          `bson:"modifiedAt"`
}

type LeadMedia struct {
	ID           int64     `bson:"id"`
	Type         string    `bson:"type"`
	Title        string    `bson:"title"`
	Date         time.Time `bson:"date"`
	Language     string    `bson:"language"`
	ImageURL     string    `bson:"imageUrl"`
	LastModified time.Time `bson:"lastModified"`
}
