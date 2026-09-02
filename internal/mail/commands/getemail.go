package commands

import (
	"context"
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/api"
)

// ErrEmailNotFound is returned when the thread comes back without the email
// that was asked for.
var ErrEmailNotFound = errors.New("email not found")

// GetEmail returns one email in full, with its body decrypted when it is
// encrypted and the account holds the keys.
//
// The API has no endpoint for a single email: the thread endpoint takes any
// email ID and returns every message in its thread, so this asks for the email
// itself and picks it out of the result.
//
// The ID that comes back is not always the one that was asked for. A message
// that lives in more than one folder has a distinct ID per copy, and the thread
// returns only one of them, so a single-message thread is taken to be the
// message that was requested.
//
// An email whose body cannot be decrypted is still returned, carrying its
// envelope; the error explains why. Callers that only want the message can
// ignore it.
func GetEmail(ctx context.Context, client Client, token, emailID string, account Account) (api.EmailResponseDto, error) {
	thread, err := client.GetThread(ctx, token, emailID)
	if err != nil {
		return api.EmailResponseDto{}, fmt.Errorf("get email %s: %w", emailID, err)
	}

	for _, email := range thread {
		if email.Id == emailID {
			return decryptBody(email, account)
		}
	}
	if len(thread) == 1 {
		return decryptBody(thread[0], account)
	}
	return api.EmailResponseDto{}, fmt.Errorf("get email %s: %w", emailID, ErrEmailNotFound)
}

// GetThread returns every email in the thread the given email belongs to,
// ordered chronologically.
func GetThread(ctx context.Context, client Client, token, emailID string) ([]api.EmailResponseDto, error) {
	thread, err := client.GetThread(ctx, token, emailID)
	if err != nil {
		return nil, fmt.Errorf("get thread of %s: %w", emailID, err)
	}
	return thread, nil
}
