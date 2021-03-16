//go:build func

package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/nglogic/go-application-guide/internal/app/bikerental"
	"github.com/nglogic/go-application-guide/pkg/api/bikerentalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestListReservations tests "ListReservations" API endpoint.
func TestListReservations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbdata := startDB(t)
	weather := NewMockWeatherService(ctrl)
	incidents := NewMockBikeIncidentsService(ctrl)
	testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

	// Create a customer.
	customer := createCustomer(t, testSetup, bikerental.CustomerTypeIndividual)

	// Create a bike.
	bike := createBike(t, testSetup)

	// Create some reservations.
	resValue := 990
	resDiscount := 10
	reservations := make(map[string]*bikerental.Reservation)
	for i := 1; i <= 5; i++ {
		starts := time.Now().Add(time.Hour * 24 * time.Duration(i))
		ends := starts.Add(time.Hour)
		r := createReservation(t, testSetup, *customer, bikerental.ReservationStatusApproved, *bike, starts, ends, resValue, resDiscount)
		reservations[r.ID] = r
	}

	// List reservations using API.
	listResp, err := testSetup.apiClient.ListReservations(context.Background(), &bikerentalv1.ListReservationsRequest{
		BikeId:    bike.ID,
		StartTime: timestamppb.New(time.Now()),
		EndTime:   timestamppb.New(time.Now().AddDate(1, 0, 0)),
	})
	require.NoError(t, err)
	require.Len(t, listResp.Reservations, len(reservations))
	for _, respRes := range listResp.Reservations {
		r, ok := reservations[respRes.Id]
		assert.True(t, ok)
		checkAPIReservation(t, r, respRes)
	}
}

// TestListReservationsTimeRange checks if "ListReservations" API endpoint returns valid reservations for time ranges "around" the reservation time.
func TestListReservationsTimeRange(t *testing.T) {
	dbdata := startDB(t)

	startDate := time.Now().Add(time.Hour * 24)
	endDate := startDate.Add(time.Hour)

	tests := []struct {
		name            string
		from            time.Time
		to              time.Time
		wantReservation bool
	}{
		{
			name:            "time range before the reservation",
			from:            startDate.Add(-time.Hour),
			to:              startDate.Add(-time.Minute),
			wantReservation: false,
		},
		{
			name:            "time range overlapping with reservation's start date",
			from:            startDate.Add(-time.Minute),
			to:              startDate.Add(time.Minute),
			wantReservation: true,
		},
		{
			name:            "time range overlapping with reservation's end date",
			from:            endDate.Add(-time.Minute),
			to:              endDate.Add(time.Minute),
			wantReservation: true,
		},
		{
			name:            "time range after the reservation",
			from:            endDate.Add(time.Minute),
			to:              endDate.Add(time.Hour),
			wantReservation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)
			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create a customer.
			customer := createCustomer(t, testSetup, bikerental.CustomerTypeBusiness)

			// Create a bike.
			bike := createBike(t, testSetup)

			// Create a reservation.
			reservation := createReservation(t, testSetup, *customer, bikerental.ReservationStatusApproved, *bike, startDate, endDate, 1000, 10)

			// List reservations using API.
			listResp, err := testSetup.apiClient.ListReservations(context.Background(), &bikerentalv1.ListReservationsRequest{
				BikeId:    bike.ID,
				StartTime: timestamppb.New(tt.from),
				EndTime:   timestamppb.New(tt.to),
			})
			require.NoError(t, err)
			if tt.wantReservation {
				assert.NotEmpty(t, listResp.Reservations)
				checkAPIReservation(t, reservation, listResp.Reservations[0])
			} else {
				assert.Empty(t, listResp.Reservations)
			}
		})
	}
}

// TestCreateReservation tests "CreateReservation" API endpoint.
func TestCreateReservation(t *testing.T) {
	t.Parallel()

	dbdata := startDB(t)
	starts := time.Now().Add(time.Hour * 24)
	ends := starts.Add(time.Hour)
	locationLat := 52.229675
	locationLong := 21.012230

	tests := []struct {
		name           string
		customerExists bool
	}{
		{
			name:           "existing customer",
			customerExists: true,
		},
		{
			name:           "new customer",
			customerExists: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)
			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create a bike.
			bike := createBike(t, testSetup)

			// We expect service to fetch weather data for reservation location.
			weather.EXPECT().
				GetWeather(gomock.Any(), bikerental.WeatherRequest{
					Location: bikerental.Location{
						Lat:  float64(float32(locationLat)),
						Long: float64(float32(locationLong)),
					},
				}).
				Return(&bikerental.Weather{
					Temperature: 5,
				}, nil).
				Times(1)

			// We expect service to fetch incidents data for reservation location.
			incidents.EXPECT().
				GetIncidents(gomock.Any(), gomock.AssignableToTypeOf(bikerental.BikeIncidentsRequest{})).
				Return(&bikerental.BikeIncidentsInfo{
					NumberOfIncidents: 10,
				}, nil).
				Times(1)

			// Create the reservation using API.
			var customer *bikerental.Customer
			var createCustomerReq *bikerentalv1.Customer
			if tt.customerExists {
				// Create customer in db first.
				customer = createCustomer(t, testSetup, bikerental.CustomerTypeIndividual)
				createCustomerReq = &bikerentalv1.Customer{Id: customer.ID}
			} else {
				// New customer data
				createCustomerReq = &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Surname:   gofakeit.LastName(),
						Email:     gofakeit.Email(),
					},
				}
			}
			createResp, err := testSetup.apiClient.CreateReservation(context.Background(), &bikerentalv1.CreateReservationRequest{
				BikeId:   bike.ID,
				Customer: createCustomerReq,
				Location: &bikerentalv1.Location{
					Lat:  float32(locationLat),
					Long: float32(locationLong),
				},
				StartTime: timestamppb.New(starts),
				EndTime:   timestamppb.New(ends),
			})
			require.NoError(t, err)

			t.Cleanup(func() {
				err := testSetup.dbadapter.Reservations().Delete(context.Background(), createResp.Reservation.Id)
				require.NoError(t, err)
			})

			assert.Equal(t, bikerentalv1.ReservationStatus_RESERVATION_STATUS_APPROVED, createResp.Status)
			assert.Greater(t, createResp.Reservation.TotalValue, int32(0), "reservation value has to be >0")
			assert.Greater(t, createResp.Reservation.AppliedDiscount, int32(0), "expected applied discount >0 (based on temperature or incidents)")

			expectedReservation := &bikerental.Reservation{
				ID:              createResp.Reservation.Id,
				Status:          bikerental.ReservationStatusApproved,
				Bike:            *bike,
				StartTime:       starts,
				EndTime:         ends,
				TotalValue:      int(createResp.Reservation.TotalValue),
				AppliedDiscount: int(createResp.Reservation.AppliedDiscount),
			}
			if tt.customerExists {
				expectedReservation.Customer = *customer
			} else {
				expectedReservation.Customer = bikerental.Customer{
					ID:        createResp.Reservation.Customer.Id,
					Type:      bikerental.CustomerTypeIndividual,
					FirstName: createResp.Reservation.Customer.Data.FirstName,
					Surname:   createResp.Reservation.Customer.Data.Surname,
					Email:     createResp.Reservation.Customer.Data.Email,
				}
			}
			checkAPIReservation(t, expectedReservation, createResp.Reservation)

			if tt.customerExists {
				checkAPICustomer(t, customer, createResp.Reservation.Customer)
			}

			checkAPIBike(t, bike, createResp.Reservation.Bike)
		})
	}
}

func TestCreateReservation_Discounts(t *testing.T) {
	t.Parallel()

	lightBike := bikerental.Bike{
		ID:           uuid.NewString(),
		ModelName:    "Light model",
		Weight:       10,
		PricePerHour: 1000,
	}
	heavyBike := bikerental.Bike{
		ID:           uuid.NewString(),
		ModelName:    "Heavy model",
		Weight:       16,
		PricePerHour: 1000,
	}
	heaviestBike := bikerental.Bike{
		ID:           uuid.NewString(),
		ModelName:    "Super-heavy model",
		Weight:       50,
		PricePerHour: 1000,
	}

	testLocation := bikerentalv1.Location{Lat: 52.229675, Long: 21.012230}

	dbdata := startDB(t)

	tests := []struct {
		name string
		req  *bikerentalv1.CreateReservationRequest

		weather    *bikerental.Weather
		weatherErr error

		incidents    *bikerental.BikeIncidentsInfo
		incidentsErr error

		wantValue    int
		wantDiscount int
	}{
		// Discounts for business customers.
		{
			name: "business customer, high reservation value, want 5%",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_BUSINESS,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 10 hours * 1000 = 10000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 34)),
			},
			wantValue:    0.95 * 10000,
			wantDiscount: 0.05 * 10000,
		},
		{
			name: "business customer, res value too low, want 0",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_BUSINESS,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 3 hours * 1000 = 3000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 27)),
			},
			wantValue:    3000,
			wantDiscount: 0,
		},

		// Discounts for individual customers, based on weight.
		{
			name: "light bike, individual customer with high reservation value, want 0",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			wantValue:    1000,
			wantDiscount: 0,
		},
		{
			name: "heavy bike, individual customer, want discount",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   heavyBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			wantValue:    1000 - (16-15)*0.01*1000,
			wantDiscount: (16 - 15) * 0.01 * 1000,
		},
		{
			name: "super heavy bike, individual customer, want discount but no more than 20%",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   heaviestBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			wantValue:    1000 - 20*0.01*1000,
			wantDiscount: 20 * 0.01 * 1000,
		},
		{
			name: "super heavy bike, business customer, want 0",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_BUSINESS,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   heaviestBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			wantValue:    1000,
			wantDiscount: 0,
		},

		// Discounts for individual customers, based on temperature.
		{
			name: "moderate temperature, individual customer, want 0",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			weather:      &bikerental.Weather{Temperature: 10},
			wantValue:    1000,
			wantDiscount: 0,
		},
		{
			name: "low temperature, individual customer, want 5%",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			weather:      &bikerental.Weather{Temperature: 5},
			wantValue:    0.95 * 1000,
			wantDiscount: 0.05 * 1000,
		},

		// Discounts for individual customers, based on incidents data.
		{
			name: "low number of incidents, individual customer, want 0",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			incidents: &bikerental.BikeIncidentsInfo{
				Proximity:         10,
				NumberOfIncidents: 2,
			},
			wantValue:    1000,
			wantDiscount: 0,
		},
		{
			name: "moderate number of incidents, individual customer, want 5%",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			incidents: &bikerental.BikeIncidentsInfo{
				Proximity:         10,
				NumberOfIncidents: 3,
			},
			wantValue:    0.95 * 1000,
			wantDiscount: 0.05 * 1000,
		},
		{
			name: "high number of incidents, individual customer, want 10%",
			req: &bikerentalv1.CreateReservationRequest{
				Customer: &bikerentalv1.Customer{
					Data: &bikerentalv1.CustomerData{
						Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
						FirstName: gofakeit.FirstName(),
						Email:     gofakeit.Email(),
					},
				},
				Location: &testLocation,
				BikeId:   lightBike.ID,

				// Res value: 1 hour * 1000 = 1000
				StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
				EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
			},
			incidents: &bikerental.BikeIncidentsInfo{
				Proximity:         10,
				NumberOfIncidents: 5,
			},
			wantValue:    0.9 * 1000,
			wantDiscount: 0.1 * 1000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			incidents := NewMockBikeIncidentsService(ctrl)
			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create test bikes.
			_ = createSpecificBike(t, testSetup, lightBike)
			_ = createSpecificBike(t, testSetup, heavyBike)
			_ = createSpecificBike(t, testSetup, heaviestBike)

			// We expect service to fetch weather data for reservation location.
			weather.EXPECT().
				GetWeather(gomock.Any(), gomock.Any()).
				Return(tt.weather, tt.weatherErr).
				Times(1)

			// We expect service to fetch incidents data for reservation location.
			incidents.EXPECT().
				GetIncidents(gomock.Any(), gomock.AssignableToTypeOf(bikerental.BikeIncidentsRequest{})).
				Return(tt.incidents, tt.incidentsErr).
				Times(1)

			createResp, err := testSetup.apiClient.CreateReservation(context.Background(), tt.req)
			require.NoError(t, err)

			t.Cleanup(func() {
				err := testSetup.dbadapter.Reservations().Delete(context.Background(), createResp.Reservation.Id)
				require.NoError(t, err)
			})

			assert.Equal(t, bikerentalv1.ReservationStatus_RESERVATION_STATUS_APPROVED, createResp.Status)
			assert.EqualValues(t, tt.wantValue, createResp.Reservation.TotalValue)
			assert.EqualValues(t, tt.wantDiscount, createResp.Reservation.AppliedDiscount)
		})
	}
}

func TestCreateReservation_Validation(t *testing.T) {
	t.Parallel()

	lightBike := bikerental.Bike{
		ID:           uuid.NewString(),
		ModelName:    "Light model",
		Weight:       10,
		PricePerHour: 1000,
	}

	newValidRequest := func() *bikerentalv1.CreateReservationRequest {
		return &bikerentalv1.CreateReservationRequest{
			Customer: &bikerentalv1.Customer{
				Data: &bikerentalv1.CustomerData{
					Type:      bikerentalv1.CustomerType_CUSTOMER_TYPE_INDIVIDUAL,
					FirstName: gofakeit.FirstName(),
					Email:     gofakeit.Email(),
				},
			},
			Location:  &bikerentalv1.Location{Lat: 52.229675, Long: 21.012230},
			BikeId:    lightBike.ID,
			StartTime: timestamppb.New(time.Now().Add(time.Hour * 24)),
			EndTime:   timestamppb.New(time.Now().Add(time.Hour * 25)),
		}
	}

	dbdata := startDB(t)

	tests := []struct {
		name              string
		newReq            func() *bikerentalv1.CreateReservationRequest
		wantValidationErr bool
	}{
		{
			name: "valid reservation",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				return newValidRequest()
			},
			wantValidationErr: false,
		},
		{
			name: "invalid customer type",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				req := newValidRequest()
				req.Customer.Data.Type = bikerentalv1.CustomerType_CUSTOMER_TYPE_UNKNOWN
				return req
			},
			wantValidationErr: true,
		},
		{
			name: "empty location",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				req := newValidRequest()
				req.Location = nil
				return req
			},
			wantValidationErr: true,
		},
		{
			name: "empty bike id",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				req := newValidRequest()
				req.BikeId = ""
				return req
			},
			wantValidationErr: true,
		},
		{
			name: "start and end time the same",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				req := newValidRequest()
				req.StartTime = timestamppb.New(time.Now().Add(time.Hour * 24))
				req.EndTime = timestamppb.New(time.Now().Add(time.Hour * 24))
				return req
			},
			wantValidationErr: true,
		},
		{
			name: "start time after end time",
			newReq: func() *bikerentalv1.CreateReservationRequest {
				req := newValidRequest()
				req.StartTime = timestamppb.New(time.Now().Add(time.Hour * 25))
				req.EndTime = timestamppb.New(time.Now().Add(time.Hour * 24))
				return req
			},
			wantValidationErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			weather := NewMockWeatherService(ctrl)
			weather.EXPECT().
				GetWeather(gomock.Any(), gomock.Any()).
				Return(nil, nil).
				MaxTimes(1)

			incidents := NewMockBikeIncidentsService(ctrl)
			incidents.EXPECT().
				GetIncidents(gomock.Any(), gomock.AssignableToTypeOf(bikerental.BikeIncidentsRequest{})).
				Return(nil, nil).
				MaxTimes(1)

			testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

			// Create test bike.
			_ = createSpecificBike(t, testSetup, lightBike)

			resp, err := testSetup.apiClient.CreateReservation(context.Background(), tt.newReq())
			if !tt.wantValidationErr {
				require.NoError(t, err)
				assert.NotNil(t, resp)

				t.Cleanup(func() {
					err := testSetup.dbadapter.Reservations().Delete(context.Background(), resp.Reservation.Id)
					require.NoError(t, err)
				})

				return
			}

			require.Error(t, err)
			assert.Nil(t, resp)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

// TestCancelReservation tests "CancelReservation" API endpoint.
func TestCancelReservation(t *testing.T) {
	t.Parallel()

	starts := time.Now().Add(time.Hour * 24)
	ends := starts.Add(time.Hour)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbdata := startDB(t)
	weather := NewMockWeatherService(ctrl)
	incidents := NewMockBikeIncidentsService(ctrl)
	testSetup := newFunctionalTestSetup(t, dbdata, weather, incidents)

	// Create a customer.
	customer := createCustomer(t, testSetup, bikerental.CustomerTypeIndividual)

	// Create a bike.
	bike := createBike(t, testSetup)

	// Create a reservation.
	reservation := createReservation(t, testSetup, *customer, bikerental.ReservationStatusApproved, *bike, starts, ends, 1000, 10)

	assert.Equal(t, bikerental.ReservationStatusApproved, reservation.Status, "initial reservation has 'approved' status")

	// Cancel it.
	_, err := testSetup.apiClient.CancelReservation(context.Background(), &bikerentalv1.CancelReservationRequest{
		Id:     reservation.ID,
		BikeId: bike.ID,
	})
	require.NoError(t, err)

	// Fetch canceled reservation from db and check the status.
	reservation, err = testSetup.dbadapter.Reservations().Get(context.Background(), reservation.ID)
	require.NoError(t, err)
	assert.Equal(t, reservation.Status, bikerental.ReservationStatusCanceled)

	// Cancel it again - the operation should be idempotent.
	_, err = testSetup.apiClient.CancelReservation(context.Background(), &bikerentalv1.CancelReservationRequest{
		Id:     reservation.ID,
		BikeId: bike.ID,
	})
	require.NoError(t, err)
}

// checkAPIReservation compares domain reservation object and API reservation object.
func checkAPIReservation(t *testing.T, res *bikerental.Reservation, responseRes *bikerentalv1.Reservation) {
	t.Helper()

	assert.Equal(t, res.ID, responseRes.Id)
	assert.Contains(
		t,
		strings.ToLower(responseRes.Status.String()),
		string(res.Status),
	)
	assert.EqualValues(t, res.AppliedDiscount, responseRes.AppliedDiscount)
	assert.EqualValues(t, res.TotalValue, responseRes.TotalValue)
	assert.InDelta(t, res.StartTime.Unix(), responseRes.StartTime.AsTime().Unix(), 10)
	assert.InDelta(t, res.EndTime.Unix(), responseRes.EndTime.AsTime().Unix(), 10)

	assert.Equal(t, res.Customer.ID, responseRes.Customer.Id)
	assert.Equal(t, res.Customer.Email, responseRes.Customer.Data.Email)
	assert.Equal(t, res.Customer.FirstName, responseRes.Customer.Data.FirstName)
	assert.Equal(t, res.Customer.Surname, responseRes.Customer.Data.Surname)
	assert.EqualValues(t, res.Customer.Type, responseRes.Customer.Data.Type)

	assert.Equal(t, res.Bike.ID, responseRes.Bike.Id)
	assert.Equal(t, res.Bike.ModelName, responseRes.Bike.Data.ModelName)
	assert.EqualValues(t, res.Bike.PricePerHour, responseRes.Bike.Data.PricePerHour)
	assert.EqualValues(t, res.Bike.Weight, responseRes.Bike.Data.Weight)
}

// checkAPICustomer compares domain customer object and API customer object.
func checkAPICustomer(t *testing.T, customer *bikerental.Customer, apiCustomer *bikerentalv1.Customer) {
	t.Helper()

	assert.Equal(t, customer.ID, apiCustomer.Id)
	assert.Equal(t, customer.Email, apiCustomer.Data.Email)
	assert.Equal(t, customer.FirstName, apiCustomer.Data.FirstName)
	assert.Equal(t, customer.Surname, apiCustomer.Data.Surname)
	assert.EqualValues(t, customer.Type, apiCustomer.Data.Type)
}
