package bikerental

import (
	"context"

	"github.com/nglogic/go-example-project/internal/app"
)

// Bike represents a bike for rent.
type Bike struct {
	ID           string
	ModelName    string
	Weight       float64
	PricePerHour float64
}

// Validate validates bike data.
func (b *Bike) Validate() error {
	if b.ModelName == "" {
		return app.NewValidationError("empty model name")
	}
	if b.Weight == 0 {
		return app.NewValidationError("empty weight")
	}

	return nil
}

// BikeService manages bikes.
type BikeService interface {
	List(context.Context) ([]Bike, error)
	Get(ctx context.Context, id string) (*Bike, error)
	Add(context.Context, Bike) (*Bike, error)
	Update(ctx context.Context, id string, b Bike) error
	Delete(ctx context.Context, id string) error
}