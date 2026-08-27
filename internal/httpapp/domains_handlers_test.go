package httpapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestDomainProjectAPIIntegration(t *testing.T) {
	server, client, database := newTestApp(t)
	defer server.Close()

	// 1. Authenticate as admin
	csrf := loginTestUser(t, client, server.URL, database)

	// 2. Query initial domains
	res, err := client.Get(server.URL + "/api/v1/domains")
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list domains status=%d want %d", res.StatusCode, http.StatusOK)
	}

	var listResult struct {
		Domains []domainDTO `json:"domains"`
	}
	if err := json.NewDecoder(res.Body).Decode(&listResult); err != nil {
		t.Fatalf("decode list domains: %v", err)
	}

	// 3. Create a new Domain
	createBody, _ := json.Marshal(map[string]any{
		"stable_key":  "eng-team",
		"slug":        "engineering",
		"name":        "Engineering",
		"description": "Core engineering documentation",
		"icon":        "code",
		"sort_order":  1,
	})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/domains", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", csrf)
	createRes, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("create domain status=%d want %d", createRes.StatusCode, http.StatusCreated)
	}

	var createDomainResult struct {
		Domain domainDTO `json:"domain"`
	}
	if err := json.NewDecoder(createRes.Body).Decode(&createDomainResult); err != nil {
		t.Fatalf("decode create domain: %v", err)
	}
	if createDomainResult.Domain.Slug != "engineering" {
		t.Fatalf("expected slug engineering, got %s", createDomainResult.Domain.Slug)
	}

	// 4. Create a Project under the new Domain
	prjBody, _ := json.Marshal(map[string]any{
		"domain_id":   createDomainResult.Domain.ID,
		"stable_key":  "eng-backend",
		"slug":        "backend",
		"name":        "Backend Services",
		"description": "Go microservices and data pipelines",
		"access_mode": "inherit",
		"sort_order":  1,
	})
	prjReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/projects", bytes.NewReader(prjBody))
	prjReq.Header.Set("Content-Type", "application/json")
	prjReq.Header.Set("X-CSRF-Token", csrf)
	prjRes, err := client.Do(prjReq)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	defer prjRes.Body.Close()
	if prjRes.StatusCode != http.StatusCreated {
		t.Fatalf("create project status=%d want %d", prjRes.StatusCode, http.StatusCreated)
	}

	var createProjectResult struct {
		Project projectDTO `json:"project"`
	}
	if err := json.NewDecoder(prjRes.Body).Decode(&createProjectResult); err != nil {
		t.Fatalf("decode create project: %v", err)
	}
	if createProjectResult.Project.Slug != "backend" {
		t.Fatalf("expected slug backend, got %s", createProjectResult.Project.Slug)
	}

	// 5. Query projects under the domain
	listPrjRes, err := client.Get(server.URL + fmt.Sprintf("/api/v1/domains/%d/projects", createDomainResult.Domain.ID))
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	defer listPrjRes.Body.Close()
	if listPrjRes.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d want %d", listPrjRes.StatusCode, http.StatusOK)
	}
}

func TestDomainProjectAPIUnauthorized(t *testing.T) {
	server, _, _ := newTestApp(t)
	defer server.Close()

	// Anonymous client without session
	anonClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	res, err := anonClient.Get(server.URL + "/api/v1/domains")
	if err != nil {
		t.Fatalf("anonymous get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther && res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous request should redirect or return 401, got status=%d", res.StatusCode)
	}
}
