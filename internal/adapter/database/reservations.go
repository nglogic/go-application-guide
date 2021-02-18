package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nglogic/go-example-project/internal/app"
	"github.com/sirupsen/logrus"
)

// Customer type enums for customer table.
const (
	customerTypeBusiness   = "business"
	customerTypeIndividual = "individual"
)

// ReservationsRepository manages reservation data in db.
type ReservationsRepository struct {
	parent *Adapter
	db     *sqlx.DB
	log    logrus.FieldLogger
}

// CreateReservation creates new reservation in db.
// Bike id must be provided.
// If customer doesn't exists, it is created with reservation.
func (r *ReservationsRepository) CreateReservation(ctx context.Context, reservation app.Reservation) (*app.Reservation, error) {
	if err := r.checkReservationData(reservation); err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	if err != nil {
		return nil, fmt.Errorf("creating postgresql transaction: %w", err)
	}
	defer rollbackTx(ctx, tx, r.log) // This will be noop after successful commit.

	bike, err := r.parent.Bikes().Get(ctx, reservation.Bike.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid bike: %w", err)
	}
	reservation.Bike = *bike

	available, err := r.checkAvailability(ctx, tx, reservation.Bike.ID, reservation.StartTime, reservation.EndTime)
	if err != nil {
		return nil, fmt.Errorf("checking bike availability: %w", err)
	}
	if !available {
		return nil, app.NewConflictError("bike not available")
	}

	if reservation.Customer.ID != "" {
		if err := r.checkCustomerExists(ctx, tx, reservation.Customer.ID); err != nil {
			return nil, fmt.Errorf("invalid customer: %w", err)
		}
	} else {
		id, err := r.parent.Customers().AddInTx(ctx, tx, reservation.Customer)
		if err != nil {
			return nil, fmt.Errorf("creating customer: %w", err)
		}
		reservation.Customer.ID = id
	}

	if err := r.createReservation(ctx, tx, reservation); err != nil {
		return nil, fmt.Errorf("creating reservation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commiting postgres reservation transaction: %w", err)
	}

	return &reservation, nil
}

// GetBikeAvailability returns true if bike with given id is available for rent in given time range.
func (r *ReservationsRepository) GetBikeAvailability(ctx context.Context, bikeID string, startTime, endTime time.Time) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("creating postgresql transaction: %w", err)
	}
	defer func() {
		_ = tx.Commit()
	}()

	return r.checkAvailability(ctx, tx, bikeID, startTime, endTime)
}

func (r *ReservationsRepository) checkAvailability(ctx context.Context, tx *sqlx.Tx, bikeID string, startTime, endTime time.Time) (bool, error) {
	var count int
	rows, err := tx.NamedQuery(
		`SELECT count(*) from reservations WHERE 
			bike_id = :bike_id
			AND start_time < :end_time
			AND end_time > :start_time
		`,
		map[string]interface{}{
			"bike_id":    bikeID,
			"start_time": startTime,
			"end_time":   endTime,
		},
	)
	if err != nil {
		return false, fmt.Errorf("querying for conflicting reservations in postgresql: %w", err)
	}
	rows.Next()
	if err := rows.Scan(&count); err != nil {
		return false, fmt.Errorf("scanning postgresql query result: %w", err)
	}

	if count > 0 {
		return false, nil
	}
	return true, nil
}

func (r *ReservationsRepository) checkReservationData(reservation app.Reservation) error {
	if reservation.ID == "" {
		return errors.New("reservation id is empty")
	}
	if reservation.Customer.ID == "" && reservation.Customer.Email == "" {
		return errors.New("customer id or email must be set")
	}
	if reservation.Bike.ID == "" {
		return errors.New("bike id is empty")
	}
	return nil
}

func (r *ReservationsRepository) checkCustomerExists(ctx context.Context, tx *sqlx.Tx, id string) error {
	err := tx.GetContext(
		ctx,
		nil,
		`SELECT id from customers WHERE id=:id`,
		id,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return app.ErrNotFound
		}
		return fmt.Errorf("fetching customer by id from postgresql: %w", err)
	}
	return nil
}

func (r *ReservationsRepository) createReservation(ctx context.Context, tx *sqlx.Tx, reservation app.Reservation) error {
	m := newReservationModel(reservation)
	_, err := r.db.NamedExecContext(
		ctx,
		`insert into reservations (id, bike_id, customer_id, start_time, end_time)
		values (:id, :bike_id, :customer_id, :start_time, :end_time)`,
		m,
	)
	if err != nil {
		return fmt.Errorf("inserting reservation row into postgres: %w", err)
	}
	return nil
}

type reservationModel struct {
	ID         string    `db:"id"`
	BikeID     string    `db:"bike_id"`
	CustomerID string    `db:"customer_id"`
	StartTime  time.Time `db:"start_time"`
	EndTime    time.Time `db:"end_time"`
}

func newReservationModel(ar app.Reservation) reservationModel {
	return reservationModel{
		ID:         ar.ID,
		BikeID:     ar.Bike.ID,
		CustomerID: ar.Customer.ID,
		StartTime:  ar.StartTime,
		EndTime:    ar.EndTime,
	}
}