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
	tier := SubscriptionTiers[string(plan)]
	return tier.canCreateProject(storageUsed, int(blogCount))
}

func CanConfigureCustomDomain(s *model.Store, sesh *session.Session) (bool, error) {
	/* get user's tier features */
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.CustomDomain, nil
}

func CanViewAnalytics(s *model.Store, sesh *session.Session) (bool, error) {
	/* get user's tier features */
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.Analytics, nil
}

func CanUploadImages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.Images, nil
}

func CanUseTheme(s *model.Store, theme string, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.canUseTheme(theme), nil
}

func CanHaveEmailSubscribers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.EmailSubscribers, nil
}

func CanHaveLikes(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.Likes, nil
}

func CanHaveComments(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.Comments, nil
}

func CanHaveTeamMembers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.TeamMembers, nil
}

func CanHavePasswordProtectedPages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.PasswordProtectedPages, nil
}

func CanHaveDownloadablePdfPages(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.PasswordProtectedPages, nil
}

func CanHavePaidSubscribers(s *model.Store, sesh *session.Session) (bool, error) {
	plan, err := s.GetUserSubscriptionByID(context.TODO(), sesh.GetUserID())
	if err != nil {
		return false, fmt.Errorf("error getting user subscription: %w", err)
	}
	tier := SubscriptionTiers[string(plan)]
	return tier.PaidSubscribers, nil
}

type SubscriptionFeatures struct {
	Projects               int
	Storage                int
	VisitorsPerMonth       int
	CustomDomain           bool
	Themes                 []string
	CodeStyle              []string
	Images                 bool
	EmailSubscribers       bool
	Analytics              bool
	Rss                    bool
	Likes                  bool
	Comments               bool
	TeamMembers            bool
	PasswordProtectedPages bool
	DownloadablePdfPages   bool
	PaidSubscribers        bool
}

var SubscriptionTiers = map[string]SubscriptionFeatures{
	"Scout": {
		Projects:               1,
		Storage:                24, /* 24MB */
		VisitorsPerMonth:       1000,
		Themes:                 []string{"lit"},
		CodeStyle:              []string{"lit"},
		CustomDomain:           false,
		Images:                 false,
		EmailSubscribers:       false,
		Analytics:              false,
		Rss:                    false,
		Likes:                  false,
		Comments:               false,
		TeamMembers:            false,
		PasswordProtectedPages: false,
		DownloadablePdfPages:   false,
		PaidSubscribers:        false,
	},
	"Wayfarer": {
		Projects:               1,
		Storage:                256, /* 256MB */
		VisitorsPerMonth:       5000,
		Themes:                 []string{"lit", "latex"},
		CodeStyle:              []string{"lit", "latex"},
		CustomDomain:           true,
		Images:                 true,
		EmailSubscribers:       true,
		Analytics:              true,
		Rss:                    true,
		Likes:                  true,
		Comments:               true,
		TeamMembers:            false,
		PasswordProtectedPages: false,
		DownloadablePdfPages:   false,
		PaidSubscribers:        false,
	},
	"Voyager": {
		Projects:               3,
		Storage:                1024, /* 1GB */
		VisitorsPerMonth:       10000,
		Themes:                 []string{"lit", "latex"},
		CodeStyle:              []string{"lit", "latex"},
		CustomDomain:           true,
		Images:                 true,
		EmailSubscribers:       true,
		Analytics:              true,
		Rss:                    true,
		Likes:                  true,
		Comments:               true,
		TeamMembers:            true,
		PasswordProtectedPages: true,
		DownloadablePdfPages:   false,
		PaidSubscribers:        false,
	},
	"Pathfinder": {
		Projects:               10,
		Storage:                10240, /* 10GB */
		VisitorsPerMonth:       100000,
		CustomDomain:           true,
		Themes:                 []string{"lit", "latex"},
		CodeStyle:              []string{"lit", "latex"},
		Images:                 true,
		EmailSubscribers:       true,
		Analytics:              true,
		Rss:                    true,
		Likes:                  true,
		Comments:               true,
		TeamMembers:            true,
		PasswordProtectedPages: true,
		DownloadablePdfPages:   true,
		PaidSubscribers:        true,
	},
}

func (s *SubscriptionFeatures) canCreateProject(bytesUsed int64, blogCount int) (bool, error) {
	if blogCount >= s.Projects {
		return false, fmt.Errorf(
			"site count %d excess max %d for plan",
			blogCount,
			s.Projects,
		)
	}
	maxBytes := megabytesToBytes(int64(s.Storage))
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
	for _, t := range s.Themes {
		if t == theme {
			return true
		}
	}
	return false
}

func OrderedSubscriptionTiers() []SubscriptionFeatures {
	var tiers []SubscriptionFeatures
	tiers = append(tiers, SubscriptionTiers["Scout"])
	tiers = append(tiers, SubscriptionTiers["Wayfarer"])
	tiers = append(tiers, SubscriptionTiers["Voyager"])
	tiers = append(tiers, SubscriptionTiers["Pathfinder"])
	return tiers
}
