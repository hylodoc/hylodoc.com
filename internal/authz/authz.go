package authz

import (
	"context"
	"fmt"

	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

func CanCreateSite(s *model.Store, sesh *session.Session) (bool, error) {
	/* get user's storage footprint */
	storageUsed, err := UserStorageUsed(s, sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error calculating user storage used: %w", err)
	}
	/* get user's site count */
	blogCount, err := s.CountBlogsByUserID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user project count: %w", err)
	}
	/* get user's tier features */
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.canCreateProject(storageUsed, int(blogCount))
}

func CanViewAnalytics(s *model.Store, sesh *session.Session) (bool, error) {
	/* get user's tier features */
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.analytics, nil
}

func CanUploadImages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.images, nil
}

func CanUseTheme(s *model.Store, theme string, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.canUseTheme(theme), nil
}

func CanHaveEmailSubscribers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.emailSubscribers, nil
}

func CanHaveLikes(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.likes, nil
}

func CanHaveComments(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.comments, nil
}

func CanHaveTeamMembers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.teamMembers, nil
}

func CanHavePasswordProtectedPages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.passwordProtectedPages, nil
}

func CanHaveDownloadablePdfPages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.passwordProtectedPages, nil
}

func CanHavePaidSubscribers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := subscriptionTiers[string(plan)]
	return tier.paidSubscribers, nil
}

type SubscriptionFeatures struct {
	projects               int
	storage                int
	visitorsPerMonth       int
	customDomain           bool
	themes                 []string
	codeStyle              []string
	images                 bool
	emailSubscribers       bool
	analytics              bool
	rss                    bool
	likes                  bool
	comments               bool
	teamMembers            bool
	passwordProtectedPages bool
	downloadablePdfPages   bool
	paidSubscribers        bool
}

var subscriptionTiers = map[string]SubscriptionFeatures{
	"Scout": {
		projects:               1,
		storage:                24, /* 24MB */
		visitorsPerMonth:       1000,
		themes:                 []string{"lit"},
		codeStyle:              []string{"lit"},
		customDomain:           false,
		images:                 false,
		emailSubscribers:       false,
		analytics:              false,
		rss:                    false,
		likes:                  false,
		comments:               false,
		teamMembers:            false,
		passwordProtectedPages: false,
		downloadablePdfPages:   false,
		paidSubscribers:        false,
	},
	"Wayfarer": {
		projects:               1,
		storage:                256, /* 256MB */
		visitorsPerMonth:       5000,
		themes:                 []string{"lit", "latex"},
		codeStyle:              []string{"lit", "latex"},
		customDomain:           true,
		images:                 true,
		emailSubscribers:       true,
		analytics:              true,
		rss:                    true,
		likes:                  true,
		comments:               true,
		teamMembers:            false,
		passwordProtectedPages: false,
		downloadablePdfPages:   false,
		paidSubscribers:        false,
	},
	"Voyager": {
		projects:               3,
		storage:                1024, /* 1GB */
		visitorsPerMonth:       10000,
		themes:                 []string{"lit", "latex"},
		codeStyle:              []string{"lit", "latex"},
		customDomain:           true,
		images:                 true,
		emailSubscribers:       true,
		analytics:              true,
		rss:                    true,
		likes:                  true,
		comments:               true,
		teamMembers:            true,
		passwordProtectedPages: true,
		downloadablePdfPages:   false,
		paidSubscribers:        false,
	},
	"Pathfinder": {
		projects:               10,
		storage:                10240, /* 10GB */
		visitorsPerMonth:       100000,
		customDomain:           false,
		themes:                 []string{"lit", "latex"},
		codeStyle:              []string{"lit", "latex"},
		images:                 true,
		emailSubscribers:       true,
		analytics:              true,
		rss:                    true,
		likes:                  true,
		comments:               true,
		teamMembers:            true,
		passwordProtectedPages: true,
		downloadablePdfPages:   true,
		paidSubscribers:        true,
	},
}

func (s *SubscriptionFeatures) canCreateProject(bytesUsed int64, blogCount int) (bool, error) {
	if blogCount >= s.projects {
		return false, fmt.Errorf(
			"site count %d excess max %d for plan",
			blogCount,
			s.projects,
		)
	}
	maxBytes := megabytesToBytes(int64(s.storage))
	if bytesUsed > maxBytes {
		return false, fmt.Errorf(
			"used %d of %d bytes",
			bytesUsed,
			maxBytes,
		)
	}
	return true, nil
}

func megabytesToBytes(mb int64) int64 {
	return mb * 1024 * 1024
}

func (s *SubscriptionFeatures) canUseTheme(theme string) bool {
	for _, t := range s.themes {
		if t == theme {
			return true
		}
	}
	return false
}
