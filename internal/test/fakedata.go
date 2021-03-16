package test

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/google/uuid"
	"github.com/nglogic/go-application-guide/internal/app"
	"github.com/nglogic/go-application-guide/internal/app/bikerental"
	"github.com/stretchr/testify/require"
)

// createBike inserts new bike into test db and returns its reference.
func createBike(t *testing.T, setup *testSetup) *bikerental.Bike {
	t.Helper()

	return createSpecificBike(t, setup, bikerental.Bike{
		ID:           uuid.NewString(),
		ModelName:    gofakeit.Username(),
		PricePerHour: 100 * int(gofakeit.Price(1, 1000)),
		Weight:       gofakeit.Float64Range(1, 50),
	})
}

func createSpecificBike(t *testing.T, setup *testSetup, b bikerental.Bike) *bikerental.Bike {
	t.Helper()

	err := setup.dbadapter.Bikes().Create(context.Background(), b)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := setup.dbadapter.Bikes().Delete(context.Background(), b.ID)
		require.True(t, err == nil || app.IsNotFoundError(err))
	})

	return &b
}

// createCustomer inserts new customer into test db and returns its reference.
func createCustomer(t *testing.T, setup *testSetup, ct bikerental.CustomerType) *bikerental.Customer {
	t.Helper()

	c := bikerental.Customer{
		ID:        uuid.NewString(),
		Type:      ct,
		FirstName: gofakeit.FirstName(),
		Surname:   gofakeit.LastName(),
		Email:     gofakeit.Email(),
	}

	err := setup.dbadapter.Customers().Create(context.Background(), c)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := setup.dbadapter.Customers().Delete(context.Background(), c.ID)
		require.True(t, err == nil || app.IsNotFoundError(err))
	})

	return &c
}

// createReservation inserts new reservation into test db and returns its reference.
func createReservation(
	t *testing.T,
	setup *testSetup,
	customer bikerental.Customer,
	status bikerental.ReservationStatus,
	bike bikerental.Bike,
	starts time.Time,
	ends time.Time,
	value int,
	discount int,
) *bikerental.Reservation {
	t.Helper()

	r := bikerental.Reservation{
		ID:              uuid.NewString(),
		Status:          status,
		Customer:        customer,
		Bike:            bike,
		StartTime:       starts,
		EndTime:         ends,
		TotalValue:      value,
		AppliedDiscount: discount,
	}

	createdRes, err := setup.dbadapter.Reservations().Create(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, bikerental.ReservationStatusApproved, createdRes.Status)

	t.Cleanup(func() {
		err := setup.dbadapter.Reservations().Delete(context.Background(), createdRes.ID)
		require.True(t, err == nil || app.IsNotFoundError(err))
	})

	return createdRes
}
