package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Nutrition as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "nutrition:read", Name: "See what you eat", Description: "Read your food diary (each day's meals with calories, protein, carbs and fat), your water, your goals, your foods and saved meals, and how your week is going — for your journal, your health app, dashboards and Pocket AI."},
	{Scope: "nutrition:write", Name: "Log food", Description: "Log what you ate (\"two eggs and toast for breakfast\") and water, and change or remove the entries that app logged itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "nutrition.json", Kind: core.KindPocket, Title: "Your foods, saved meals, food diary, water and day notes"},
		{Path: "/api/v1/export.csv", File: "diary.csv", Kind: core.KindCSV, Title: "Your food diary, one row per food eaten"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", Title: "Adds the foods, saved meals, diary, water and notes from another box; what is already here is left alone"},
		{Path: "/api/v1/import", Kind: core.KindCSV, ContentType: "text/csv", Title: "Adds a food diary as CSV (this app's own, MyFitnessPal or Cronometer); rows already here are skipped"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(overview|summary|week|trends|entries|foods|foods/search|meals|day/\d{4}-\d{2}-\d{2})$`), "nutrition:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(entries|foods|meals)/\d+$`), "nutrition:read"},
	{"POST", regexp.MustCompile(`^/api/v1/entries/log$`), "nutrition:write"},
	{"POST", regexp.MustCompile(`^/api/v1/water/\d{4}-\d{2}-\d{2}/add$`), "nutrition:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/entries/\d+$`), "nutrition:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "nutrition" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
