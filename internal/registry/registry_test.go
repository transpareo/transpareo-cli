package registry

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/spec"
)

func loadFixture(t *testing.T) *Registry {
	t.Helper()
	data, err := os.ReadFile("testdata/fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestLoadFixture(t *testing.T) {
	reg := loadFixture(t)
	if reg.Version != "9.9.9" {
		t.Errorf("version = %q", reg.Version)
	}
	ids := []string{}
	for _, op := range reg.Operations {
		ids = append(ids, op.ID)
	}
	want := "login_session logout_session list_things create_thing " +
		"bulk_create_things validate_thing delete_thing"
	if strings.Join(ids, " ") != want {
		t.Errorf("ids = %v", ids)
	}
	if g := reg.Groups(); strings.Join(g,
		" ") != "reference-data session things" {
		t.Errorf("groups = %v", g)
	}
}

func TestParametersAndSecurity(t *testing.T) {
	reg := loadFixture(t)
	list := reg.Find("list_things")
	if list == nil {
		t.Fatal("list_things missing")
	}
	if list.Group != "reference-data" || list.Tag != "Reference Data" ||
		list.Method != "GET" {
		t.Errorf("op = %+v", list)
	}
	if len(list.QueryParams) != 2 || list.QueryParams[0].Name != "page" ||
		list.QueryParams[1].Name != "q" {
		t.Errorf("query params = %+v", list.QueryParams)
	}
	if !list.Public || strings.Join(list.Security, ",") != "oauth2,userToken" ||
		list.UserOnly {
		t.Errorf("security = %v public %v userOnly %v", list.Security,
			list.Public, list.UserOnly)
	}
	if len(list.Permission) != 0 || list.Permission == nil {
		t.Errorf("permission = %#v", list.Permission)
	}
	if !list.ReadOnly() || list.ResponseStatus != "200" {
		t.Errorf("read only %v status %q", list.ReadOnly(), list.ResponseStatus)
	}

	del := reg.Find("delete_thing")
	if len(del.PathParams) != 1 || del.PathParams[0].Name != "id" ||
		!del.PathParams[0].Required ||
		del.PathParams[0].Description != "Record ID" {
		t.Errorf("path params = %+v", del.PathParams)
	}
	if !del.Destructive || del.ReadOnly() ||
		strings.Join(del.Permission, ",") != "thing_access,thing_write" {
		t.Errorf("delete = %+v", del)
	}
	if del.ResponseStatus != "204" || del.ResponseSchema != nil {
		t.Errorf("response = %q %s", del.ResponseStatus, del.ResponseSchema)
	}

	create := reg.Find("create_thing")
	if strings.Join(create.Security, ",") != "oauth2" || create.Public {
		t.Errorf("inherited security = %v", create.Security)
	}
	if strings.Join(create.Permission, ",") != "thing_write" {
		t.Errorf("string permission = %v", create.Permission)
	}
}

func TestRequestBodyResolution(t *testing.T) {
	reg := loadFixture(t)
	create := reg.Find("create_thing")
	if !create.RequestRequired ||
		create.RequestContentType != "application/json" {
		t.Errorf("body = %+v", create)
	}
	var schema map[string]any
	if err := json.Unmarshal(create.RequestBody, &schema); err != nil {
		t.Fatal(err)
	}
	allOf, _ := schema["allOf"].([]any)
	if len(allOf) != 2 {
		t.Fatalf("allOf = %v", schema)
	}
	thing, _ := allOf[0].(map[string]any)
	props, _ := thing["properties"].(map[string]any)
	parent, _ := props["parent"].(map[string]any)
	if parent["$ref"] != "#/components/schemas/Thing" {
		t.Errorf("a recursive reference must stay in place: %v", parent)
	}
	tags, _ := props["tags"].(map[string]any)
	items, _ := tags["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("nested reference not resolved: %v", tags)
	}
	if string(create.RequestExample) != `{"id":"new","note":"hi"}` {
		t.Errorf("example = %s", create.RequestExample)
	}
	if string(create.ResponseExample) != `{"id":"t1"}` {
		t.Errorf("schema example = %s", create.ResponseExample)
	}
	if create.ResponseStatus != "201" {
		t.Errorf("first success response = %q", create.ResponseStatus)
	}

	validate := reg.Find("validate_thing")
	if !validate.Safe || !validate.ReadOnly() ||
		string(validate.RequestExample) != `{"a":1}` {
		t.Errorf("validate = %+v example %s", validate, validate.RequestExample)
	}

	bulk := reg.Find("bulk_create_things")
	if !bulk.NDJSON || bulk.ResponseContentType != "application/x-ndjson" {
		t.Errorf("bulk = %+v", bulk)
	}

	login := reg.Find("login_session")
	if login.RequestContentType != "application/x-www-form-urlencoded" ||
		!login.Public || login.UserOnly {
		t.Errorf("login = %+v", login)
	}
	logout := reg.Find("logout_session")
	if !logout.UserOnly {
		t.Errorf("logout must be user only: %+v", logout)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load([]byte("nope")); err == nil {
		t.Error("invalid JSON must fail")
	}
	noID := `{"info":{"version":"1"},"paths":{"/x":{"get":{"responses":{}}}}}`
	if _, err := Load([]byte(noID)); err == nil ||
		!strings.Contains(err.Error(), "operationId") {
		t.Errorf("err = %v", err)
	}
	dangling := `{"info":{"version":"1"},"paths":{"/x":{"get":{"operationId":"a",
		"parameters":[{"$ref":"#/components/parameters/nope"}],"responses":{}}}}}`
	if _, err := Load([]byte(dangling)); err == nil ||
		!strings.Contains(err.Error(), "dangling") {
		t.Errorf("err = %v", err)
	}
	dup := `{"info":{"version":"1"},"paths":{
		"/x":{"get":{"operationId":"a","responses":{}}},
		"/y":{"get":{"operationId":"a","responses":{}}}}}`
	if _, err := Load([]byte(dup)); err == nil ||
		!strings.Contains(err.Error(), "twice") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadVendoredSpecification(t *testing.T) {
	reg, err := Load(spec.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Version != spec.Version() {
		t.Errorf("version = %q, want %q", reg.Version, spec.Version())
	}
	if len(reg.Operations) < 100 {
		t.Errorf("only %d operations", len(reg.Operations))
	}
	me := reg.Find("get_me")
	if me == nil || me.Method != "GET" || me.Path != "/me" || me.UserOnly {
		t.Fatalf("get_me = %+v", me)
	}
	if reg.Find("logout_session") == nil ||
		!reg.Find("logout_session").UserOnly {
		t.Error("logout_session must be user only")
	}
	if !reg.Find("void_dpp").Destructive || !reg.Find("validate_dpp").Safe {
		t.Error("extensions not read from the vendored specification")
	}
	for _, op := range reg.Operations {
		if op.Group == "" || op.Method == "" {
			t.Errorf("%s has no group or method", op.ID)
		}
		if op.RequestBody != nil && !json.Valid(op.RequestBody) {
			t.Errorf("%s has an invalid request schema", op.ID)
		}
		if strings.Contains(string(op.RequestBody),
			`"$ref":"#/components/responses`) {
			t.Errorf("%s keeps a response reference in its body", op.ID)
		}
	}
}

func TestMatch(t *testing.T) {
	reg, err := Load(spec.JSON)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ method, path, want string }{
		{"get", "/me", "get_me"},
		{"GET", "/api/me", "get_me"},
		{"GET", "me?reload=1", "get_me"},
		{"GET", "/dpps/ABC", "get_dpp"},
		{"GET", "/dpps/requirements", "get_dpp_requirements"},
		{"POST", "/dpps/validate", "validate_dpp"},
		{"POST", "/dpps/ABC/void", "void_dpp"},
		{"POST", "/dpps/bulk/validate", "validate_dpps_bulk"},
		{"GET", "/dpps/bulk/507f", "get_bulk_task"},
		{"DELETE", "/webhooks/1", "delete_webhook"},
		{"GET", "/nothing/here", ""},
		{"PATCH", "/me", ""},
		{"GET", "/dpps/", "list_dpps"},
	}
	for _, tc := range cases {
		got := ""
		if op := reg.Match(tc.method, tc.path); op != nil {
			got = op.ID
		}
		if got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
