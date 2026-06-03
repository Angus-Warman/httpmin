package parserequest

import (
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type User struct {
	Name     string
	Email    *string // Optional
	Role     int
	Archived *bool
	Flags    []string
}

func TestParseFormData(t *testing.T) {
	email := "alice@example.com"

	tests := []struct {
		name        string
		form        url.Values
		want        User
		expectError bool
	}{
		{
			name: "all fields",
			form: url.Values{"name": {"Alice"}, "email": {email}, "role": {"1"}},
			want: User{Name: "Alice", Email: &email, Role: 1},
		},
		{
			name: "missing optional field",
			form: url.Values{"name": {"Bob"}, "role": {"2"}},
			want: User{Name: "Bob", Role: 2},
		},
		{
			name:        "invalid role (non-integer)",
			form:        url.Values{"name": {"Carol"}, "role": {"admin"}},
			expectError: true,
		},
		{
			name: "multiple values",
			form: url.Values{"name": {"Dave"}, "role": {"3"}, "flags": {"advisor", "warden"}},
			want: User{Name: "Dave", Role: 3, Flags: []string{"advisor", "warden"}},
		},
		{
			name:        "empty string is nil",
			form:        url.Values{"name": {""}, "role": {"0"}},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(tc.form.Encode())
			r := httptest.NewRequest("POST", "/", body)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			got, err := As[User](r)

			if tc.expectError {
				if err == nil {
					t.Fatalf("should've failed but didn't")
				}

				t.Log(err)

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
