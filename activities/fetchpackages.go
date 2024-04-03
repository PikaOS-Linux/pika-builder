package activities

import (
	"context"
	"pkbldr/packages"
)

func FetchPackages(ctx context.Context) error {
	err := packages.ProcessPackages()
	if err != nil {
		return err
	}
	return nil
}
