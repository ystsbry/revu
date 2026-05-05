package github

import "testing"

func TestSlugFromPRURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "same-repo PR",
			url:  "https://github.com/ystsbry/revu/pull/10",
			want: "ystsbry/revu",
		},
		{
			name: "fork PR url still points at base repo",
			url:  "https://github.com/ystsbry/revu/pull/42",
			want: "ystsbry/revu",
		},
		{
			name: "trailing slash tolerated",
			url:  "https://github.com/owner/repo/pull/1/",
			want: "owner/repo",
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "not a PR url",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "issues path is not a PR",
			url:     "https://github.com/owner/repo/issues/3",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := slugFromPRURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got slug=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("slugFromPRURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
