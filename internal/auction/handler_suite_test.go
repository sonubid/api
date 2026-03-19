package auction_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type handlerSuite struct {
	suite.Suite
}

// TestHandlerSuite is the testify suite runner for auction route handlers.
func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(handlerSuite))
}
