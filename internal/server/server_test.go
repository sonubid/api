package server_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sonubid/api/internal/server"
)

const (
	waitTimeout  = 2 * time.Second
	pollInterval = 10 * time.Millisecond
)

type serverSuite struct {
	suite.Suite
}

func TestServerSuite(t *testing.T) {
	suite.Run(t, new(serverSuite))
}

func (s *serverSuite) TestStartReturnsErrInvalidAddressWhenAddrIsEmpty() {
	logger := discardLogger()

	err := server.Start(context.Background(), logger, http.NewServeMux(), "")

	s.ErrorIs(err, server.ErrInvalidAddress)
}

func (s *serverSuite) TestStartReturnsErrNilLoggerWhenLoggerIsNil() {
	err := server.Start(context.Background(), nil, http.NewServeMux(), ":0")

	s.ErrorIs(err, server.ErrNilLogger)
}

func (s *serverSuite) TestStartReturnsErrNilHandlerWhenHandlerIsNil() {
	logger := discardLogger()

	err := server.Start(context.Background(), logger, nil, ":0")

	s.ErrorIs(err, server.ErrNilHandler)
}

func (s *serverSuite) TestStartShutsDownCleanlyOnContextCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeAddr(s.T())
	logger := discardLogger()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx, logger, http.NewServeMux(), addr)
	}()

	waitForServer(s.T(), addr)

	cancel()

	select {
	case err := <-errCh:
		s.NoError(err)
	case <-time.After(waitTimeout):
		s.Fail("server did not shut down within timeout")
	}
}

func (s *serverSuite) TestStartReturnsErrorWhenAddressAlreadyInUse() {
	ln, err := net.Listen("tcp", ":0") //nolint:gosec,noctx // test-only: binding to all interfaces is intentional
	s.Require().NoError(err)
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	logger := discardLogger()

	err = server.Start(context.Background(), logger, http.NewServeMux(), addr)

	s.Error(err)
	s.NotErrorIs(err, server.ErrInvalidAddress)
	s.NotErrorIs(err, server.ErrNilLogger)
	s.NotErrorIs(err, server.ErrNilHandler)
}

func (s *serverSuite) TestStartServesRequestsWhileRunning() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeAddr(s.T())
	logger := discardLogger()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		_ = server.Start(ctx, logger, mux, addr)
	}()

	waitForServer(s.T(), addr)

	resp, err := http.Get("http://" + addr + "/health") //nolint:noctx // test-only convenience
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	s.Equal(http.StatusOK, resp.StatusCode)
}
