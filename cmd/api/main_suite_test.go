package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	testSyncInterval          = 10 * time.Millisecond
	testCleanupInterval       = 15 * time.Millisecond
	testSyncWaitTimeout       = time.Second
	testAuctionIDExisting     = "auction-existing"
	testAuctionIDNew          = "auction-new"
	testAuctionIDInvalid      = ""
	testExistingStartingPrice = uint64(100)
	testExistingCurrentBid    = uint64(500)
	testProviderStartingPrice = uint64(50)
	testProviderCurrentBid    = uint64(60)
	testEnvSyncInterval       = "20ms"
	testEnvCleanupInterval    = "25ms"
	testEnvInvalidDuration    = "abc"
	testEnvZeroDuration       = "0s"
)

type mainSuite struct {
	suite.Suite
}

func TestMainSuite(t *testing.T) {
	suite.Run(t, new(mainSuite))
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
