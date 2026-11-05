package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-nutrition/internal/store"
)

// ---- settings ----

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

// publicSettings is what one person sees: their settings with the secrets
// masked and, per Pocket app, the mode in effect (<app>_mode), whether it
// works and how (<app>_via: core | direct | "") and why not (<app>_detail).
func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	out := map[string]any{
		"core_configured": s.core.Configured(), "core_registered": s.coreStatus()["registered"],
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart,
		"energy_unit": st.EnergyUnit, "weight_unit": st.WeightUnit,
		"kcal_goal": st.KcalGoal, "protein_goal": st.ProteinGoal, "carbs_goal": st.CarbsGoal, "fat_goal": st.FatGoal, "fiber_goal": st.FiberGoal, "water_goal": st.WaterGoal, "glass_ml": st.GlassMl,
		"sex": st.Sex, "birth_year": st.BirthYear, "height_cm": st.HeightCm, "weight_kg": st.WeightKg, "activity": st.Activity, "aim": st.Aim, "aim_rate": st.AimRate,
		"online_lookup":  st.OnlineLookup,
		"notify_morning": st.NotifyMorning, "notify_evening": st.NotifyEvening, "notify_week": st.NotifyWeek, "notify_hour": st.NotifyHour, "evening_hour": st.EveningHour,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"health_send": st.HealthSend, "health_weight": st.HealthWeight, "health_burn": st.HealthBurn, "habit_id": st.HabitID, "habit_name": st.HabitName, "habit_rule": st.HabitRule,
		"kitchen_plan": st.KitchenPlan, "fitness_days": st.FitnessDays, "places_meals": st.PlacesMeals,
		"ai_review": st.AIReview, "demo_apps": s.demoApps && u != nil && u.ID == s.store.OwnerID(ctx),
		"ai_provider": st.AIProvider, "ai_pocket_model": st.AIPocketModel, "openai_api_key": mask(st.OpenAIKey), "openai_model": st.OpenAIModel, "openai_base_url": st.OpenAIBaseURL,
		"setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL,
	}
	for _, a := range pocketApps {
		mode, via, detail := s.connStatus(ctx, u, st, a)
		out[a.slug+"_mode"], out[a.slug+"_via"], out[a.slug+"_detail"] = mode, via, detail
		out[a.slug+"_configured"] = via != ""
		_, url, token, public := connSettings(st, a)
		out[a.slug+"_url"], out[a.slug+"_token"], out[a.slug+"_public_url"] = url, mask(token), public
		if public == "" && via != "" {
			public = s.resolvePublic(ctx, a.slug)
		}
		out[a.slug+"_open_url"] = public
	}
	return out
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), userFrom(r.Context()), st))
}

// handlePutSettings saves the caller's personal settings. The box-wide title
// is only taken from an admin.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r.Context())
	isAdmin := me.Role == "admin"
	cur, err := s.store.GetSettings(r.Context(), me.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	raw := map[string]any{}
	var in store.Settings
	keys, err := decodeKeys(r, &raw)
	if err != nil || remarshal(raw, &in) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	bad := func(msg string) { writeErr(w, http.StatusBadRequest, msg) }
	if keys["title"] && isAdmin {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["week_start"] && (in.WeekStart == 1 || in.WeekStart == 7 || in.WeekStart == 6) {
		cur.WeekStart = in.WeekStart
	}
	if keys["energy_unit"] && oneOf(in.EnergyUnit, []string{"kcal", "kj"}) {
		cur.EnergyUnit = in.EnergyUnit
	}
	if keys["weight_unit"] && oneOf(in.WeightUnit, []string{"kg", "lb"}) {
		cur.WeightUnit = in.WeightUnit
	}
	goal := func(k string, dst *int, v, maxV int, what string) bool {
		if !keys[k] {
			return true
		}
		if v < 0 || v > maxV {
			bad(fmt.Sprintf("the %s goal is 0 to %d", what, maxV))
			return false
		}
		*dst = v
		return true
	}
	if !goal("kcal_goal", &cur.KcalGoal, in.KcalGoal, 10000, "calorie") || !goal("protein_goal", &cur.ProteinGoal, in.ProteinGoal, 500, "protein") ||
		!goal("carbs_goal", &cur.CarbsGoal, in.CarbsGoal, 1000, "carbohydrate") || !goal("fat_goal", &cur.FatGoal, in.FatGoal, 500, "fat") ||
		!goal("fiber_goal", &cur.FiberGoal, in.FiberGoal, 200, "fibre") || !goal("water_goal", &cur.WaterGoal, in.WaterGoal, 10000, "water") {
		return
	}
	if keys["glass_ml"] {
		if in.GlassMl < 50 || in.GlassMl > 2000 {
			bad("a glass is 50 to 2000 ml")
			return
		}
		cur.GlassMl = in.GlassMl
	}
	if keys["sex"] && oneOf(in.Sex, []string{"", "female", "male"}) {
		cur.Sex = in.Sex
	}
	if keys["birth_year"] {
		if in.BirthYear != 0 && (in.BirthYear < 1900 || in.BirthYear > time.Now().Year()-10) {
			bad("the year of birth is 1900 or later")
			return
		}
		cur.BirthYear = in.BirthYear
	}
	if keys["height_cm"] {
		if in.HeightCm != 0 && (in.HeightCm < 100 || in.HeightCm > 250) {
			bad("your height is 100 to 250 cm")
			return
		}
		cur.HeightCm = in.HeightCm
	}
	if keys["weight_kg"] {
		if in.WeightKg != 0 && (in.WeightKg < 25 || in.WeightKg > 350) {
			bad("your weight is 25 to 350 kg")
			return
		}
		cur.WeightKg = in.WeightKg
	}
	if keys["activity"] && oneOf(in.Activity, []string{"low", "light", "moderate", "high", "very"}) {
		cur.Activity = in.Activity
	}
	if keys["aim"] && oneOf(in.Aim, []string{"lose", "keep", "gain"}) {
		cur.Aim = in.Aim
	}
	if keys["aim_rate"] {
		if in.AimRate < 0 || in.AimRate > 1 {
			bad("up to 1 kg a week")
			return
		}
		cur.AimRate = in.AimRate
	}
	if keys["habit_rule"] && oneOf(in.HabitRule, []string{"logged", "goal"}) {
		cur.HabitRule = in.HabitRule
	}
	for k, dst := range map[string]*int{"notify_hour": &cur.NotifyHour, "evening_hour": &cur.EveningHour} {
		if !keys[k] {
			continue
		}
		v := in.NotifyHour
		if k == "evening_hour" {
			v = in.EveningHour
		}
		if v < 0 || v > 23 {
			bad("the hour is 0 to 23")
			return
		}
		*dst = v
	}
	if keys["habit_id"] {
		cur.HabitID = in.HabitID
		if in.HabitID == 0 {
			cur.HabitName = ""
		}
	}
	secret := func(k string, dst *string, v string) {
		if keys[k] && !strings.Contains(v, "••") {
			*dst = strings.TrimSpace(v)
		}
	}
	url := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	text := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimSpace(v)
		}
	}
	for k, dst := range map[string]*bool{"online_lookup": &cur.OnlineLookup, "notify_morning": &cur.NotifyMorning, "notify_evening": &cur.NotifyEvening, "notify_week": &cur.NotifyWeek,
		"health_send": &cur.HealthSend, "health_weight": &cur.HealthWeight, "health_burn": &cur.HealthBurn, "kitchen_plan": &cur.KitchenPlan,
		"fitness_days": &cur.FitnessDays, "places_meals": &cur.PlacesMeals, "ai_review": &cur.AIReview} {
		if v, ok := raw[k].(bool); ok && keys[k] {
			*dst = v
		}
	}
	text("habit_name", &cur.HabitName, in.HabitName)
	text("notify_url", &cur.NotifyURL, in.NotifyURL)
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	// each Pocket app: mode, URL, token, public URL
	inMap := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			inMap[k] = sv
		}
	}
	for _, a := range pocketApps {
		c := cur.Conns[a.slug]
		if v := inMap[a.slug+"_mode"]; keys[a.slug+"_mode"] && (v == modeCore || v == modeDirect || v == modeOff) {
			c.Mode = v
		}
		url(a.slug+"_url", &c.URL, inMap[a.slug+"_url"])
		secret(a.slug+"_token", &c.Token, inMap[a.slug+"_token"])
		url(a.slug+"_public_url", &c.PublicURL, inMap[a.slug+"_public_url"])
		if c != (store.Conn{}) {
			cur.Conns[a.slug] = c
		}
	}
	if keys["ai_provider"] {
		if !validAIProvider(in.AIProvider) {
			bad("AI is automatic, Pocket AI or OpenAI")
			return
		}
		cur.AIProvider = in.AIProvider
	}
	if keys["ai_pocket_model"] {
		if !validPocketModel(in.AIPocketModel) {
			bad("unknown model")
			return
		}
		cur.AIPocketModel = in.AIPocketModel
	}
	secret("openai_api_key", &cur.OpenAIKey, in.OpenAIKey)
	text("openai_model", &cur.OpenAIModel, in.OpenAIModel)
	url("openai_base_url", &cur.OpenAIBaseURL, in.OpenAIBaseURL)
	if err := s.store.SaveUserSettings(r.Context(), me.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if isAdmin && keys["title"] {
		if err := s.store.SaveBoxSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	for k := range keys {
		if isModeKey(k) {
			s.reportModesSoon()
			break
		}
	}
	// what comes from the other apps may have changed
	for _, p := range []string{"weight", "hday", "plan", "fit", "visits", "recipes"} {
		_ = s.store.CacheDelete(r.Context(), fmt.Sprintf("%s:%d:", p, me.ID))
	}
	if keys["health_send"] || keys["habit_id"] || keys["habit_rule"] {
		s.changed(me.ID)
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), me, cur))
}

// handleIntegrationTest checks a connection with the caller's own saved
// settings (or the URL/key typed into the form).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which  string `json:"which"`
		URL    string `json:"url"`
		Key    string `json:"key"`
		Format string `json:"format"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := actorID(r.Context())
	st, _ := s.store.GetSettings(r.Context(), uid)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	keyOr := func(saved string) string {
		if strings.Contains(in.Key, "••") || in.Key == "" {
			return saved
		}
		return in.Key
	}
	urlOr := func(saved string) string {
		if strings.TrimSpace(in.URL) == "" {
			return saved
		}
		return strings.TrimRight(strings.TrimSpace(in.URL), "/")
	}
	var msg string
	var err error
	if a, ok := appBySlug(in.Which); ok {
		mode, savedURL, savedTok, _ := connSettings(st, a)
		typed := in.Key != "" && !strings.Contains(in.Key, "••")
		var conn *siblingConn
		if tok := keyOr(savedTok); tok != "" && (typed || s.connMode(mode, savedTok) == modeDirect) {
			base := urlOr(savedURL)
			if base == "" {
				base = s.resolveSibling(ctx, a.slug)
			}
			conn = &siblingConn{client: pocket.New(base, tok, 15*time.Second), via: modeDirect}
		} else {
			conn, err = s.clients(ctx, uid).conn(ctx, a, a.scopes...)
		}
		if err == nil {
			how := "direct connection"
			if conn.via == modeCore {
				how = "through Pocket Core"
			}
			var v string
			if v, err = conn.client.Health(ctx); err == nil {
				msg = fmt.Sprintf("%s %s, %s", a.name, v, how)
				msg += s.testDetail(ctx, a, conn)
			}
		}
	} else if in.Which == "openai" {
		c := ai.New(urlOr(st.OpenAIBaseURL), keyOr(st.OpenAIKey), st.OpenAIModel, 20*time.Second)
		var ids []string
		if ids, err = c.Models(ctx); err == nil {
			msg = fmt.Sprintf("The key works (%d models)", len(ids))
		}
	} else if in.Which == "notify" {
		f := in.Format
		if f == "" {
			f = st.NotifyFormat
		}
		n := notify.New(urlOr(st.NotifyURL), f)
		err = n.Send(ctx, notify.Message{Title: "Pocket Nutrition", Body: notifyTestBody, Priority: "info", Tags: []string{"green_salad"}})
		msg = "Test message sent via " + n.Format
	} else {
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// ---- system / backup / export ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.CountsFor(r.Context(), actorID(r.Context())),
	})
}

// handleBackup downloads the whole database, an admin-only box operation.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-nutrition-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, dst)
}
