package reservations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nglogic/go-example-project/internal/app"
)

// Service provies methods for making reservations.
type Service struct {
	discountService  app.DiscountService
	bikeService      app.BikeService
	reservationsRepo Repository
}

// NewService creates new service instance.
func NewService(discounts app.DiscountService, bikeService app.BikeService, reservationsRepo Repository) (*Service, error) {
	if discounts == nil {
		return nil, errors.New("empty discount service")
	}
	if bikeService == nil {
		return nil, errors.New("empty bike service")
	}
	if reservationsRepo == nil {
		return nil, errors.New("empty reservations repository")
	}

	return &Service{
		discountService:  discounts,
		bikeService:      bikeService,
		reservationsRepo: reservationsRepo,
	}, nil
}

// MakeReservation creates new reservation if possible.
// If creating reservation is not possible due to business logic or availability issues, this method returns valid response.
// If there are errors while processing request, returns nil and an error.
func (s *Service) MakeReservation(ctx context.Context, req app.ReservationRequest) (*app.ReservationResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// We don't trust bike pricing from request,
	// so we fetch real bike data from bike service.
	bike, err := s.fetchRealBike(ctx, req.Bike)
	if err != nil {
		return nil, err
	}
	if bike == nil {
		return &app.ReservationResponse{
			Status: app.ReservationStatusRejected,
			Reason: fmt.Sprintf("bike with id '%s' does not exists", req.Bike.ID),
		}, nil
	}

	tx, err := s.reservationsRepo.StartTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating reservation transaction: %w", err)
	}
	// Rollback will be ignored if commit is called first.
	defer tx.Rollback()

	bikeAvailable, err := s.checkBikeAvailability(tx, *bike, req.From, req.To)
	if err != nil {
		return nil, fmt.Errorf("checking bike availability: %w", err)
	}
	if !bikeAvailable {
		return &app.ReservationResponse{
			Status: app.ReservationStatusRejected,
			Reason: "bike not available in requested time range",
		}, nil
	}

	value := s.calculateReservationValue(*bike, req.From, req.To)

	discountResp, err := s.discountService.CalculateDiscount(ctx, app.DiscountRequest{
		Customer:         req.Customer,
		Location:         req.Location,
		Bike:             *bike,
		ReservationValue: value,
	})
	if err != nil {
		return nil, fmt.Errorf("checking available discounts: %w", err)
	}

	reservation := app.Reservation{
		ID:         uuid.New().String(),
		Customer:   req.Customer,
		Bike:       req.Bike,
		From:       req.From,
		To:         req.To,
		TotalValue: value - discountResp.Discount.Amount,
	}
	err = tx.CreateReservation(reservation)
	if err != nil {
		return nil, fmt.Errorf("creating reservation in repository: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commiting reservation transaction: %w", err)
	}

	return &app.ReservationResponse{
		Status:                app.ReservationStatusApproved,
		Reservation:           &reservation,
		AppliedDiscountAmount: discountResp.Discount.Amount,
	}, nil
}

func (s *Service) fetchRealBike(ctx context.Context, bike app.Bike) (*app.Bike, error) {
	if bike.ID == "" {
		return nil, errors.New("empty bike id")
	}

	existingBike, err := s.bikeService.Get(ctx, bike.ID)
	if err != nil {
		return nil, fmt.Errorf("checking bike in repository: %w", err)
	}
	return existingBike, nil
}

func (s *Service) checkBikeAvailability(tx RepositoryTransaction, bike app.Bike, from, to time.Time) (bool, error) {
	reservations, err := tx.ListReservations(bike.ID, from, to)
	if err != nil {
		return false, fmt.Errorf("listing existing reservations: %w", err)
	}
	if len(reservations) > 0 {
		return false, nil
	}

	return true, nil
}

func (s *Service) calculateReservationValue(bike app.Bike, from, to time.Time) float64 {
	return bike.PricePerHour * to.Sub(from).Hours()
}
