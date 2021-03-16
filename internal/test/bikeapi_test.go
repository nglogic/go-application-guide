//go:build func

package test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/nglogic/go-application-guide/internal/app/bikerental"
	"github.com/nglogic/go-application-guide/pkg/api/bikerentalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestListBikes tests "ListBikes" API endpoint.
func TestListBikes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbdata := startDB(t)

	tests := []struct {
		name       string
		bikesCount int
	}{
		{
			name:       "empty list",
			bikesCount: 0,
		},
		{
			name:       "5 bikes",
			bikesCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)
			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create bikes.
			bikes := make(map[string]*bikerental.Bike)
			for i := 0; i < tt.bikesCount; i++ {
				bike := createBike(t, testSetup)
				bikes[bike.ID] = bike
			}

			listResp, err := testSetup.apiClient.ListBikes(context.Background(), &emptypb.Empty{})
			require.NoError(t, err)
			require.Len(t, listResp.Bikes, tt.bikesCount)
			for _, respBike := range listResp.Bikes {
				b, ok := bikes[respBike.Id]
				assert.True(t, ok)
				checkAPIBike(t, b, respBike)
			}
		})
	}
}

// TestGetBikes tests GetBike API endpoint.
func TestGetBikes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbdata := startDB(t)
	weather := NewMockWeatherService(ctrl)
	incidents := NewMockBikeIncidentsService(ctrl)
	testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

	// Create 5 bikes.
	bikes := make(map[string]*bikerental.Bike)
	for i := 0; i < 5; i++ {
		bike := createBike(t, testSetup)
		bikes[bike.ID] = bike
	}

	// Fetch bikes by id using API.
	for id, b := range bikes {
		t.Run("fetch bikes by id", func(t *testing.T) {
			getResp, err := testSetup.apiClient.GetBike(context.Background(), &bikerentalv1.GetBikeRequest{
				Id: id,
			})
			require.NoError(t, err)
			require.NotEmpty(t, getResp)
			checkAPIBike(t, b, getResp)
		})
	}
}

// TestCreateBike tests "CreateBike" API endpoint.
func TestCreateBike(t *testing.T) {
	tests := []struct {
		name              string
		req               *bikerentalv1.CreateBikeRequest
		wantValidationErr bool
	}{
		{
			name: "don't allow empty name",
			req: &bikerentalv1.CreateBikeRequest{
				Data: &bikerentalv1.BikeData{
					ModelName:    "",
					Weight:       10,
					PricePerHour: 10,
				},
			},
			wantValidationErr: true,
		},
		{
			name: "don't allow empty weight",
			req: &bikerentalv1.CreateBikeRequest{
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					PricePerHour: 10,
				},
			},
			wantValidationErr: true,
		},
		{
			name: "create bike with valid data",
			req: &bikerentalv1.CreateBikeRequest{
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					Weight:       10,
					PricePerHour: 10,
				},
			},
			wantValidationErr: false,
		},
	}

	dbdata := startDB(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)

			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create bike using API.
			createdBike, err := testSetup.apiClient.CreateBike(context.Background(), tt.req)
			if tt.wantValidationErr {
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, st.Code(), codes.InvalidArgument)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, createdBike.Id)

			// Assert created bike is in the db.
			bikes, err := testSetup.dbadapter.Bikes().List(context.Background())
			require.NoError(t, err)
			require.NotEmpty(t, bikes)

			// Cleanup.
			t.Cleanup(func() {
				err := testSetup.dbadapter.Bikes().Delete(context.Background(), createdBike.Id)
				require.NoError(t, err)
			})

			// Check response data against database data.
			checkAPIBike(t, &bikes[0], createdBike)
		})
	}
}

// TestUpdateBike tests "CreateBike" API endpoint.
func TestUpdateBike(t *testing.T) {
	existingBikeID := uuid.NewString()
	notExistingBikeID := uuid.NewString()

	tests := []struct {
		name              string
		id                string
		bike              bikerental.Bike
		req               *bikerentalv1.UpdateBikeRequest
		wantValidationErr bool
		wantNotFoundErr   bool
	}{
		{
			name: "don't allow passing empty id",
			req: &bikerentalv1.UpdateBikeRequest{
				Id: "",
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					Weight:       10,
					PricePerHour: 10,
				},
			},
			wantValidationErr: true,
		},
		{
			name: "don't allow empty name",
			req: &bikerentalv1.UpdateBikeRequest{
				Id: existingBikeID,
				Data: &bikerentalv1.BikeData{
					ModelName:    "",
					Weight:       10,
					PricePerHour: 10,
				},
			},
			wantValidationErr: true,
		},
		{
			name: "don't allow empty weight",
			req: &bikerentalv1.UpdateBikeRequest{
				Id: existingBikeID,
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					PricePerHour: 10,
					Weight:       0,
				},
			},
			wantValidationErr: true,
		},
		{
			name: "update bike that don't exists",
			req: &bikerentalv1.UpdateBikeRequest{
				Id: notExistingBikeID,
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					Weight:       10,
					PricePerHour: 10,
				},
			},
			wantNotFoundErr: true,
		},
		{
			name: "update existing bike with valid data",
			req: &bikerentalv1.UpdateBikeRequest{
				Id: existingBikeID,
				Data: &bikerentalv1.BikeData{
					ModelName:    "test",
					Weight:       10,
					PricePerHour: 10,
				},
			},
		},
	}

	dbdata := startDB(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)
			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create initial bike to update.
			createSpecificBike(t, testSetup, bikerental.Bike{
				ID:           existingBikeID,
				ModelName:    "testModel",
				Weight:       11,
				PricePerHour: 22,
			})

			// Update bike using API.
			_, err := testSetup.apiClient.UpdateBike(context.Background(), tt.req)
			switch {
			case tt.wantValidationErr:
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, st.Code(), codes.InvalidArgument)
				return
			case tt.wantNotFoundErr:
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, st.Code(), codes.NotFound)
				return
			}

			require.NoError(t, err)

			// Assert bike was updated.
			bikes, err := testSetup.dbadapter.Bikes().List(context.Background())
			require.NoError(t, err)
			require.NotEmpty(t, bikes)
			assert.Equal(t, tt.req.Data.ModelName, bikes[0].ModelName)
			assert.EqualValues(t, tt.req.Data.PricePerHour, bikes[0].PricePerHour)
			assert.EqualValues(t, tt.req.Data.Weight, bikes[0].Weight)
		})
	}
}

// TestDeleteBike tests "DeleteBike" API endpoint.
func TestDeleteBike(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbdata := startDB(t)
	weather := NewMockWeatherService(ctrl)
	incidents := NewMockBikeIncidentsService(ctrl)
	testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

	// Create bike.
	bike := createBike(t, testSetup)

	// Delete bike by API request.
	_, err := testSetup.apiClient.DeleteBike(context.Background(), &bikerentalv1.DeleteBikeRequest{Id: bike.ID})
	require.NoError(t, err)

	// Assert no bikes are present.
	bikes, err := testSetup.dbadapter.Bikes().List(context.Background())
	require.NoError(t, err)
	require.Empty(t, bikes)
}

// checkAPIBike compares domain bike object and API bike object.
func checkAPIBike(t *testing.T, bike *bikerental.Bike, apiBike *bikerentalv1.Bike) {
	t.Helper()

	assert.Equal(t, bike.ID, apiBike.Id)
	assert.Equal(t, bike.ModelName, apiBike.Data.ModelName)
	assert.EqualValues(t, bike.PricePerHour, apiBike.Data.PricePerHour)
	assert.EqualValues(t, bike.Weight, apiBike.Data.Weight)
}
