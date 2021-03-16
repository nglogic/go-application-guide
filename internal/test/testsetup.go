package test

import (
	"context"
	"net"
	"path"
	"testing"

	"github.com/nglogic/go-application-guide/internal/adapter/database"
	bikerental "github.com/nglogic/go-application-guide/internal/app/bikerental"
	"github.com/nglogic/go-application-guide/internal/app/bikerental/bikes"
	"github.com/nglogic/go-application-guide/internal/app/bikerental/discount"
	"github.com/nglogic/go-application-guide/internal/app/bikerental/reservation"
	grpctransport "github.com/nglogic/go-application-guide/internal/transport/grpc"
	"github.com/nglogic/go-application-guide/pkg/api/bikerentalv1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

//go:generate mockgen -package=test -destination=weather_mock.go github.com/nglogic/go-application-guide/internal/app/bikerental WeatherService
//go:generate mockgen -package=test -destination=incidents_mock.go github.com/nglogic/go-application-guide/internal/app/bikerental BikeIncidentsService

// testSetup contains client to test grpc server and references to all app services useful for testing.
type testSetup struct {
	dbadapter          *database.Adapter
	bikeService        *bikes.Service
	discountService    *discount.Service
	reservationService *reservation.Service

	apiClient bikerentalv1.BikeRentalServiceClient
}

func newFunctionalTestSetup(
	t *testing.T,
	dbdata testDB,
	weatherService bikerental.WeatherService,
	incidentsService bikerental.BikeIncidentsService,
) *testSetup {
	t.Helper()

	var setup testSetup

	log := logrus.New()

	dbAdapter, err := database.NewAdapter(
		dbdata.hostPort,
		dbdata.name,
		dbdata.user,
		dbdata.pass,
		path.Join(ProjectAbsoultePath(), "configs", "postgresql"),
		log,
	)
	require.NoError(t, err)
	setup.dbadapter = dbAdapter

	bikeService, err := bikes.NewService(dbAdapter.Bikes())
	require.NoError(t, err)
	setup.bikeService = bikeService

	discountService, err := discount.NewService(weatherService, incidentsService)
	require.NoError(t, err)
	setup.discountService = discountService

	reservationService, err := reservation.NewService(
		discountService,
		bikeService,
		dbAdapter.Reservations(),
		dbAdapter.Customers(),
	)
	require.NoError(t, err)
	setup.reservationService = reservationService

	srv, err := grpctransport.NewServer(bikeService, reservationService, log)
	require.NoError(t, err)

	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		err = grpctransport.RunServer(
			ctx,
			log,
			srv,
			listener,
		)
		require.NoError(t, err)
	}()

	conn, err := grpc.DialContext(ctx, "", grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
	})

	setup.apiClient = bikerentalv1.NewBikeRentalServiceClient(conn)

	return &setup
}
