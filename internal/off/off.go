// Package off looks foods up on Open Food Facts (openfoodfacts.org), the
// free, open database of packaged foods: by barcode, or by name. No key is
// needed; requests carry the app's User-Agent as Open Food Facts asks.
package off

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Base is the Open Food Facts address (changed in tests).
var Base = "https://world.openfoodfacts.org"

// Product is a food found on Open Food Facts, its numbers per 100 g or 100 ml.
type Product struct {
	Barcode  string
	Name     string
	Brand    string
	Unit     string // g | ml
	Kcal     float64
	Protein  float64
	Carbs    float64
	Fat      float64
	Fiber    *float64
	Sugar    *float64
	SatFat   *float64
	Sodium   *float64 // mg
	Serving  string   // "1 bar (45 g)"
	ServingG float64  // its amount in the unit
	Package  float64  // the whole package in the unit
	Category string
	ImageURL string
}

// ErrNotFound means Open Food Facts does not know the barcode.
var ErrNotFound = errors.New("not on Open Food Facts")

// Client asks Open Food Facts.
type Client struct {
	UserAgent string
	HTTP      *http.Client
}

// New makes a client.
func New(userAgent string) *Client {
	return &Client{UserAgent: userAgent, HTTP: &http.Client{Timeout: 12 * time.Second}}
}

const fields = "code,product_name,product_name_en,generic_name,brands,nutriments,serving_size,serving_quantity,quantity,product_quantity,categories_tags,image_front_small_url,nutrition_data_per"

type rawProduct struct {
	Code            string             `json:"code"`
	Name            string             `json:"product_name"`
	NameEn          string             `json:"product_name_en"`
	Generic         string             `json:"generic_name"`
	Brands          string             `json:"brands"`
	Nutriments      map[string]any     `json:"nutriments"`
	ServingSize     string             `json:"serving_size"`
	ServingQuantity json.RawMessage    `json:"serving_quantity"`
	Quantity        string             `json:"quantity"`
	ProductQuantity json.RawMessage    `json:"product_quantity"`
	Categories      []string           `json:"categories_tags"`
	Image           string             `json:"image_front_small_url"`
	Extra           map[string]float64 `json:"-"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Open Food Facts did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return errors.New("Open Food Facts is busy; try again in a minute")
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("Open Food Facts answered %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Barcode looks a barcode up.
func (c *Client) Barcode(ctx context.Context, code string) (*Product, error) {
	code = strings.TrimSpace(code)
	if !regexp.MustCompile(`^\d{6,14}$`).MatchString(code) {
		return nil, errors.New("a barcode is 6 to 14 digits")
	}
	var out struct {
		Status  int        `json:"status"`
		Product rawProduct `json:"product"`
	}
	if err := c.get(ctx, "/api/v2/product/"+code+".json", url.Values{"fields": {fields}}, &out); err != nil {
		return nil, err
	}
	if out.Status != 1 {
		return nil, ErrNotFound
	}
	p := convert(out.Product)
	if p == nil {
		return nil, ErrNotFound
	}
	if p.Barcode == "" {
		p.Barcode = code
	}
	return p, nil
}

// Search finds products by name (best matches first, only ones with numbers).
func (c *Client) Search(ctx context.Context, q string, n int) ([]Product, error) {
	var out struct {
		Products []rawProduct `json:"products"`
	}
	v := url.Values{"search_terms": {q}, "search_simple": {"1"}, "action": {"process"}, "json": {"1"}, "page_size": {strconv.Itoa(n * 2)}, "fields": {fields}}
	if err := c.get(ctx, "/cgi/search.pl", v, &out); err != nil {
		return nil, err
	}
	res := []Product{}
	for _, rp := range out.Products {
		if p := convert(rp); p != nil && len(res) < n {
			res = append(res, *p)
		}
	}
	return res, nil
}

func num(m map[string]any, k string) (float64, bool) {
	switch v := m[k].(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		return f, err == nil
	}
	return 0, false
}

func rawNum(r json.RawMessage) float64 {
	var f float64
	if json.Unmarshal(r, &f) == nil {
		return f
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		f, _ = strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	}
	return f
}

func r2(v float64) float64 { return math.Round(v*100) / 100 }

// convert reads a product's numbers per 100 g / 100 ml; nil when it has none.
func convert(rp rawProduct) *Product {
	n := rp.Nutriments
	if n == nil {
		return nil
	}
	name := strings.TrimSpace(rp.Name)
	if name == "" {
		name = strings.TrimSpace(rp.NameEn)
	}
	if name == "" {
		name = strings.TrimSpace(rp.Generic)
	}
	if name == "" {
		return nil
	}
	p := &Product{Barcode: rp.Code, Name: name, Unit: "g", ImageURL: rp.Image}
	if b := strings.Split(rp.Brands, ","); len(b) > 0 {
		p.Brand = strings.TrimSpace(b[0])
	}
	kcal, ok := num(n, "energy-kcal_100g")
	if !ok {
		if kj, ok2 := num(n, "energy_100g"); ok2 { // kJ
			kcal, ok = kj/4.184, true
		}
	}
	if !ok {
		return nil
	}
	p.Kcal = math.Round(kcal)
	p.Protein, _ = num(n, "proteins_100g")
	p.Carbs, _ = num(n, "carbohydrates_100g")
	p.Fat, _ = num(n, "fat_100g")
	p.Protein, p.Carbs, p.Fat = r2(p.Protein), r2(p.Carbs), r2(p.Fat)
	opt := func(k string, scale float64) *float64 {
		if v, ok := num(n, k); ok {
			x := r2(v * scale)
			return &x
		}
		return nil
	}
	p.Fiber, p.Sugar, p.SatFat = opt("fiber_100g", 1), opt("sugars_100g", 1), opt("saturated-fat_100g", 1)
	if v := opt("sodium_100g", 1000); v != nil {
		p.Sodium = v
	} else if v := opt("salt_100g", 400); v != nil { // salt is 40% sodium
		p.Sodium = v
	}
	q := strings.ToLower(rp.Quantity + " " + rp.ServingSize)
	if strings.Contains(q, "ml") || strings.Contains(q, " l") || strings.Contains(q, "cl") {
		p.Unit = "ml"
	}
	p.ServingG = rawNum(rp.ServingQuantity)
	p.Serving = strings.TrimSpace(rp.ServingSize)
	p.Package = rawNum(rp.ProductQuantity)
	for _, c := range rp.Categories {
		if cat := category(c); cat != "" {
			p.Category = cat
		}
	}
	return p
}

// category maps an Open Food Facts category tag to one of ours.
func category(tag string) string {
	t := strings.TrimPrefix(tag, "en:")
	for k, v := range map[string]string{
		"beverages": "drinks", "dairies": "dairy", "cheeses": "dairy", "yogurts": "dairy", "breads": "bread", "cereals-and-potatoes": "grains",
		"breakfast-cereals": "grains", "snacks": "snacks", "sweet-snacks": "sweets", "chocolates": "sweets", "biscuits": "sweets",
		"meats": "meat", "fishes": "fish", "seafood": "fish", "fruits": "fruit", "vegetables": "vegetables", "legumes": "legumes",
		"nuts": "nuts", "fats": "fats", "sauces": "fats", "meals": "meals", "eggs": "eggs",
	} {
		if t == k {
			return v
		}
	}
	return ""
}
