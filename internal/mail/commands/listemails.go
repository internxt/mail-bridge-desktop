package commands

import (
	"context"
	"fmt"

	"mail-bridge-desktop/internal/api"
)

// ListEmails returns one page of email summaries from a folder.
//
// Summaries carry no bodies: use GetEmail for that.
func ListEmails(ctx context.Context, client Client, token string, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	emails, err := client.GetUserFolder(ctx, token, opts)
	if err != nil {
		return api.EmailListResponseDto{}, fmt.Errorf("list emails: %w", err)
	}
	return emails, nil
}

// ListAllEmails pages through a folder until the API runs out of emails, so
// callers that need the whole folder do not have to handle pagination.
func ListAllEmails(ctx context.Context, client Client, token string, opts api.ListEmailsOptions, account Account, onPreviewError func(error)) ([]api.EmailSummaryResponseDto, error) {
	var all []api.EmailSummaryResponseDto

	for {
		page, err := ListEmails(ctx, client, token, opts)
		if err != nil {
			return nil, err
		}

		for _, summary := range page.Emails {
			summary, err := decryptPreview(summary, account)
			if err != nil && onPreviewError != nil {
				onPreviewError(err)
			}
			all = append(all, summary)
		}

		// Stop when the API says there is no more, and also when a page comes
		// back empty: without that guard a misbehaving API would loop forever.
		if !page.HasMoreMails || len(page.Emails) == 0 {
			return all, nil
		}
		opts.Position += len(page.Emails)
	}
}
