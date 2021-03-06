// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestCloud(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Cloud")
			c.Check(a, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "cloud-foo"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.CloudResults{})
			results := result.(*params.CloudResults)
			results.Results = append(results.Results, params.CloudResult{
				Cloud: &params.Cloud{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
			})
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.Cloud(names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result_ interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Clouds")
			c.Check(a, gc.IsNil)
			c.Assert(result_, gc.FitsTypeOf, &params.CloudsResult{})
			result := result_.(*params.CloudsResult)
			result.Clouds = map[string]params.Cloud{
				"cloud-foo": {
					Type: "bar",
				},
				"cloud-baz": {
					Type:      "qux",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	clouds, err := client.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("foo"): {
			Name: "foo",
			Type: "bar",
		},
		names.NewCloudTag("baz"): {
			Name:      "baz",
			Type:      "qux",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
}

func (s *cloudSuite) TestDefaultCloud(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "DefaultCloud")
			c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
			results := result.(*params.StringResult)
			results.Result = "cloud-foo"
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.DefaultCloud()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, names.NewCloudTag("foo"))
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UserCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
			c.Assert(a, jc.DeepEquals, params.UserClouds{UserClouds: []params.UserCloud{{
				UserTag:  "user-bob",
				CloudTag: "cloud-foo",
			}}})
			*result.(*params.StringsResults) = params.StringsResults{
				Results: []params.StringsResult{{
					Result: []string{
						"cloudcred-foo_bob_one",
						"cloudcred-foo_bob_two",
					},
				}},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.UserCredentials(names.NewUserTag("bob"), names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("foo/bob/one"),
		names.NewCloudCredentialTag("foo/bob/two"),
	})
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			c.Assert(a, jc.DeepEquals, params.TaggedCredentials{
				Credentials: []params.TaggedCredential{{
					Tag: "cloudcred-foo_bob_bar",
					Credential: params.CloudCredential{
						AuthType: "userpass",
						Attributes: map[string]string{
							"username": "admin",
							"password": "adm1n",
						},
					},
				}}})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
	err := client.UpdateCredential(tag, cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "admin",
		"password": "adm1n",
	}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RevokeCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "cloudcred-foo_bob_bar",
			}}})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
	err := client.RevokeCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentials(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Credential")
			c.Assert(result, gc.FitsTypeOf, &params.CloudCredentialResults{})
			c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "cloudcred-foo_bob_bar",
			}}})
			*result.(*params.CloudCredentialResults) = params.CloudCredentialResults{
				Results: []params.CloudCredentialResult{
					{
						Result: &params.CloudCredential{
							AuthType:   "userpass",
							Attributes: map[string]string{"username": "fred"},
							Redacted:   []string{"password"},
						},
					}, {
						Result: &params.CloudCredential{
							AuthType:   "userpass",
							Attributes: map[string]string{"username": "mary"},
							Redacted:   []string{"password"},
						},
					},
				},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
	result, err := client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{
			Result: &params.CloudCredential{
				AuthType:   "userpass",
				Attributes: map[string]string{"username": "fred"},
				Redacted:   []string{"password"},
			},
		}, {
			Result: &params.CloudCredential{
				AuthType:   "userpass",
				Attributes: map[string]string{"username": "mary"},
				Redacted:   []string{"password"},
			},
		},
	})
}

func (s *cloudSuite) TestAddCloudNotInV1API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddCloud")
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})

	c.Assert(err, gc.ErrorMatches, "AddCloud\\(\\).* not implemented")
}

func (s *cloudSuite) TestAddCloudV2API(c *gc.C) {
	var called bool
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddCloud")
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestAddCredentialNotInV1API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCredential("cloudcred-acloud-user-credname",
		cloud.NewCredential(cloud.UserPassAuthType, map[string]string{}))

	c.Assert(err, gc.ErrorMatches, "AddCredential\\(\\).* not implemented")
}

func (s *cloudSuite) TestAddCredentialV2API(c *gc.C) {
	var called bool
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddCredentials")
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}

				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.AddCredential("cloudcred-acloud-user-credname",
		cloud.NewCredential(cloud.UserPassAuthType,
			map[string]string{}))

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsArgumentCheck(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{BestVersion: 2}
	client := cloudapi.NewClient(apiCaller)

	// Check supplying cloud name without credential name is invalid.
	result, err := client.CredentialContents("cloud", "", true)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "credential name must be supplied")

	// Check supplying credential name without cloud name is invalid.
	result, err = client.CredentialContents("", "credential", true)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cloud name must be supplied")
}

func (s *cloudSuite) TestCredentialContentsAll(c *gc.C) {
	expectedResults := []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "cloud-name",
					Name:     "credential-name",
					AuthType: "userpass",
					Attributes: map[string]string{
						"username": "fred",
						"password": "sekret"},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"}, // no access
				},
			},
		}, {
			Error: common.ServerError(errors.New("boom")),
		},
	}
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CredentialContents")
				c.Assert(result, gc.FitsTypeOf, &params.CredentialContentResults{})
				c.Assert(a, jc.DeepEquals, params.CloudCredentialArgs{
					IncludeSecrets: true,
				})
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: expectedResults,
				}
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("", "", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *cloudSuite) TestCredentialContentsOne(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CredentialContents")
				c.Assert(result, gc.FitsTypeOf, &params.CredentialContentResults{})
				c.Assert(a, jc.DeepEquals, params.CloudCredentialArgs{
					IncludeSecrets: true,
					Credentials: []params.CloudCredentialArg{
						{CloudName: "cloud-name", CredentialName: "credential-name"},
					},
				})
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: []params.CredentialContentResult{
						{},
					},
				}
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("cloud-name", "credential-name", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
}

func (s *cloudSuite) TestCredentialContentsGotMoreThanBargainedFor(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: []params.CredentialContentResult{
						{},
						{},
					},
				}
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("cloud-name", "credential-name", true)
	c.Assert(results, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "expected 1 result for credential \"cloud-name\" on cloud \"credential-name\", got 2")
}

func (s *cloudSuite) TestCredentialContentsServerError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return errors.New("boom")
			}),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("", "", true)
	c.Assert(results, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *cloudSuite) TestCredentialContentsNotInV2API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	_, err := client.CredentialContents("", "", true)
	c.Assert(err, gc.ErrorMatches, "CredentialContents\\(\\).* not implemented")
}
