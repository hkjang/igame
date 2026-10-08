package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// These tests use PostgreSQL and the production router/session middleware via
// newAdminListFixture. Run with IGAME_TEST_DSN set; without it they skip.
type catalogOrderFixture struct {
	adminListFixture
	prefix string
	tied   []string
}

const (
	catalogFirstNameID = "20000000-0000-0000-0000-000000000001"
	catalogLastNameID  = "00000000-0000-0000-0000-000000000001"
)

func newCatalogOrderFixture(t *testing.T) catalogOrderFixture {
	t.Helper()
	f := catalogOrderFixture{adminListFixture: newAdminListFixture(t), prefix: "catalog-" + uuid.NewString()}
	ctx := context.Background()
	var category uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO categories(slug,name) VALUES($1,$1) RETURNING id`, f.prefix).Scan(&category); err != nil {
		t.Fatal(err)
	}
	// UUID order is deliberately the reverse of insertion order. The expected
	// order below is the API contract, never an assumption about heap order.
	if _, err := f.pool.Exec(ctx, `INSERT INTO games(id,slug,name,category_id,game_url,status)
		SELECT ('10000000-0000-0000-0000-'||lpad(i::text,12,'0'))::uuid,
		$1||'-'||i,$1||'-M',$2,'/games/catalog-order/','active'
		FROM generate_series(6,1,-1) AS g(i)`, f.prefix, category); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		f.tied = append(f.tied, fmt.Sprintf("10000000-0000-0000-0000-%012d", i))
	}
	// The A/Z controls have IDs in the opposite order to their names. A is
	// outside the category; Z is not a favorite. The disabled row matches both.
	if _, err := f.pool.Exec(ctx, `INSERT INTO games(id,slug,name,category_id,game_url,status) VALUES
		($1,$3||'-a',$3||'-A',NULL,'/games/catalog-order/','active'),
		($2,$3||'-z',$3||'-Z',$4,'/games/catalog-order/','active'),
		('10000000-0000-0000-0000-000000000007',$3||'-disabled',$3||'-M',$4,'/games/catalog-order/','disabled')`,
		catalogFirstNameID, catalogLastNameID, f.prefix, category); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO favorites(user_id,game_id)
		SELECT $1,id FROM games WHERE name LIKE $2||'-%' AND id<>$3`, f.admin, f.prefix, catalogLastNameID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f catalogOrderFixture) page(t *testing.T, query string, offset int) []map[string]any {
	t.Helper()
	path := fmt.Sprintf("/api/v1/games?%s&limit=2&offset=%d", query, offset)
	req, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, res.StatusCode)
	}
	var out struct {
		Items  []map[string]any `json:"items"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Items == nil || out.Limit != 2 || out.Offset != offset || len(out.Items) > 2 {
		t.Fatalf("GET %s: invalid page envelope: %+v", path, out)
	}
	return out.Items
}

func assertCatalogOrder(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("catalog name/id order: got %v, want %v", got, want)
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] != 1 {
			t.Errorf("catalog ID %s appeared %d times, want exactly once", id, seen[id])
		}
	}
}

func TestCatalogListOrdersTiedNamesByID(t *testing.T) {
	f := newCatalogOrderFixture(t)
	all := append([]string{catalogFirstNameID}, f.tied...)
	all = append(all, catalogLastNameID)
	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{"tied names", "q=" + url.QueryEscape(f.prefix+"-M"), f.tied},
		{"name before ID", "q=" + f.prefix, all},
		{"category", "q=" + f.prefix + "&category=" + f.prefix, all[1:]},
		{"favorite", "q=" + f.prefix + "&favorite=true", all[:len(all)-1]},
		{"combined filters", "q=" + f.prefix + "&category=" + f.prefix + "&favorite=true", f.tied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for offset := 0; offset < len(tc.want); offset += 2 {
				for _, item := range f.page(t, tc.query, offset) {
					got = append(got, item["id"].(string))
					if item["status"] != "active" {
						t.Errorf("catalog returned non-active game %s", item["id"])
					}
					if tc.name == "favorite" || tc.name == "combined filters" {
						if item["favorite"] != true {
							t.Errorf("favorite filter returned non-favorite game %s", item["id"])
						}
					}
				}
			}
			assertCatalogOrder(t, got, tc.want)
			if items := f.page(t, tc.query, len(tc.want)); len(items) != 0 {
				t.Errorf("page past filtered set returned %d items", len(items))
			}
		})
	}
}

func (f catalogOrderFixture) updateDescription(t *testing.T, before map[string]any) {
	t.Helper()
	// Round-trip every editable field from the real GET so PUT preserves more
	// than just the sorting/filter fields (including true-by-default flags).
	raw, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var in gameInput
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	in.Description = "description changed during pagination"
	raw, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, f.server.URL+"/api/v1/admin/games/"+before["id"].(string), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT game: status %d, want 200", res.StatusCode)
	}
	var out struct {
		Game map[string]any `json:"game"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Game["description"] != in.Description || before["description"] == in.Description {
		t.Fatal("PUT did not change the description")
	}
	for key, value := range before {
		if key != "description" && key != "updated_at" && !reflect.DeepEqual(out.Game[key], value) {
			t.Errorf("description-only PUT changed %s: got %v, want %v", key, out.Game[key], value)
		}
	}
	var stored string
	if err := f.pool.QueryRow(context.Background(), `SELECT description FROM games WHERE id=$1`, before["id"]).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != in.Description {
		t.Fatalf("stored description %q, want %q", stored, in.Description)
	}
}

func TestCatalogListKeepsOrderAcrossDescriptionUpdate(t *testing.T) {
	f := newCatalogOrderFixture(t)
	query := "q=" + f.prefix + "&category=" + f.prefix + "&favorite=true"
	var got []string
	for _, offset := range []int{0, 2, 4} {
		items := f.page(t, query, offset)
		if len(items) != 2 {
			t.Fatalf("offset %d: got %d items, want 2", offset, len(items))
		}
		for _, item := range items {
			got = append(got, item["id"].(string))
		}
		if offset == 0 {
			// Update an already-seen row. Its name, ID, status, category and
			// favorite remain unchanged; q matches its unchanged name.
			f.updateDescription(t, items[0])
		}
	}
	assertCatalogOrder(t, got, f.tied)
	// A fresh walk also verifies that the filtered set is still the same.
	var after []string
	for _, offset := range []int{0, 2, 4, 6} {
		for _, item := range f.page(t, query, offset) {
			after = append(after, item["id"].(string))
		}
	}
	assertCatalogOrder(t, after, f.tied)
}
