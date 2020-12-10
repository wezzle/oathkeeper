package authn

import (
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"net/http"

	"github.com/ory/go-convenience/stringsx"

	"github.com/ory/oathkeeper/driver/configuration"
	"github.com/ory/oathkeeper/helper"
	"github.com/ory/oathkeeper/pipeline"
)

func init() {
	gjson.AddModifier("this", func(json, arg string) string {
		return json
	})
}

type AuthenticatorSimpleBearerFilter struct {
}

type AuthenticatorSimpleBearerConfiguration struct {
	CheckTokenURL       string                      `json:"check_token_url"`
	BearerTokenLocation *helper.BearerTokenLocation `json:"token_from"`
	PreservePath        bool                        `json:"preserve_path"`
	ExtraFrom           string                      `json:"extra_from"`
	SubjectFrom         string                      `json:"subject_from"`
}

type AuthenticatorSimpleBearer struct {
	c configuration.Provider
}

func NewAuthenticatorSimpleBearer(c configuration.Provider) *AuthenticatorSimpleBearer {
	return &AuthenticatorSimpleBearer{
		c: c,
	}
}

func (a *AuthenticatorSimpleBearer) GetID() string {
	return "simple_bearer"
}

func (a *AuthenticatorSimpleBearer) Validate(config json.RawMessage) error {
	if !a.c.AuthenticatorIsEnabled(a.GetID()) {
		return NewErrAuthenticatorNotEnabled(a)
	}

	_, err := a.Config(config)
	return err
}

func (a *AuthenticatorSimpleBearer) Config(config json.RawMessage) (*AuthenticatorSimpleBearerConfiguration, error) {
	var c AuthenticatorSimpleBearerConfiguration
	if err := a.c.AuthenticatorConfig(a.GetID(), config, &c); err != nil {
		return nil, NewErrAuthenticatorMisconfigured(a, err)
	}

	if len(c.ExtraFrom) == 0 {
		c.ExtraFrom = "extra"
	}

	if len(c.SubjectFrom) == 0 {
		c.SubjectFrom = "subject"
	}

	return &c, nil
}

func (a *AuthenticatorSimpleBearer) Authenticate(r *http.Request, session *AuthenticationSession, config json.RawMessage, _ pipeline.Rule) error {
	cf, err := a.Config(config)
	if err != nil {
		return err
	}

	token := helper.BearerTokenFromRequest(r, cf.BearerTokenLocation)
	if token == "" {
		return errors.WithStack(ErrAuthenticatorNotResponsible)
	}

	body, err := forwardRequestToSessionStore(r, cf.CheckTokenURL, cf.PreservePath)
	if err != nil {
		return err
	}

	var (
		subject string
		extra   map[string]interface{}

		subjectRaw = []byte(stringsx.Coalesce(gjson.GetBytes(body, cf.SubjectFrom).Raw, "null"))
		extraRaw   = []byte(stringsx.Coalesce(gjson.GetBytes(body, cf.ExtraFrom).Raw, "null"))
	)

	if err = json.Unmarshal(subjectRaw, &subject); err != nil {
		return helper.ErrForbidden.WithReasonf("The configured subject_from GJSON path returned an error on JSON output: %s", err.Error()).WithDebugf("GJSON path: %s\nBody: %s\nResult: %s", cf.SubjectFrom, body, subjectRaw).WithTrace(err)
	}

	if err = json.Unmarshal(extraRaw, &extra); err != nil {
		return helper.ErrForbidden.WithReasonf("The configured extra_from GJSON path returned an error on JSON output: %s", err.Error()).WithDebugf("GJSON path: %s\nBody: %s\nResult: %s", cf.ExtraFrom, body, extraRaw).WithTrace(err)
	}

	session.Subject = subject
	session.Extra = extra
	return nil
}
