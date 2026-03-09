package auction

import "errors"

var (
	ErrAuctionNotFound  = errors.New("auction not found")
	ErrBidTooLow        = errors.New("bid amount is too low")
	ErrAuctionClosed    = errors.New("auction is not active")
	ErrInvalidAuctionID = errors.New("auction ID must not be empty")
)
