package test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/stretchr/testify/require"
)

// testDB contains data for connecting to test database.
type testDB struct {
	hostPort string
	name     string
	user     string
	pass     string
}

func startDB(t *testing.T) testDB {
	t.Helper()

	var db *sql.DB
	var err error
	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "connecting to docker")

	dbname := "testdb"
	user := "postgres"
	pass := "secret"

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "9.6",
		Env: []string{
			"POSTGRES_USER=" + user,
			"POSTGRES_PASSWORD=" + pass,
			"POSTGRES_DB=" + dbname,
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	require.NoError(t, err, "starting docker resource")

	// Sometimes test cleanup fails. Be sure that test containers don't hang there for eternity.
	// Whatever happens, max lifetime of this container would be 10 minutes.
	err = resource.Expire(10 * 60)
	require.NoError(t, err, "updating docker resource expiration date")

	// Remove postgres container on test finish.
	t.Cleanup(func() {
		err := pool.Purge(resource)
		require.NoError(t, err, "removing docker container")
	})

	hostPort := resource.GetHostPort("5432/tcp")

	err = pool.Retry(func() error {
		var err error
		db, err = sql.Open("postgres", fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, pass, hostPort, dbname))
		if err != nil {
			return err
		}
		return db.Ping()
	})
	require.NoError(t, err, "connecting to postgres container")

	return testDB{
		hostPort: hostPort,
		name:     dbname,
		user:     user,
		pass:     pass,
	}
}
