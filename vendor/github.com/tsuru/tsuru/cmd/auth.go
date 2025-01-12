// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

type loginScheme struct {
	Name string
	Data map[string]string
}

type login struct {
	scheme *loginScheme
}

func nativeLogin(context *Context, client *Client) error {
	var email string
	if len(context.Args) > 0 {
		email = context.Args[0]
	} else {
		fmt.Fprint(context.Stdout, "Email: ")
		fmt.Fscanf(context.Stdin, "%s\n", &email)
	}
	fmt.Fprint(context.Stdout, "Password: ")
	password, err := PasswordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout)
	url, err := GetURL("/users/" + email + "/tokens")
	if err != nil {
		return err
	}
	b := bytes.NewBufferString(`{"password":"` + password + `"}`)
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	out := make(map[string]interface{})
	err = json.Unmarshal(result, &out)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Successfully logged in!")
	return writeToken(out["token"].(string))
}

func (c *login) getScheme() *loginScheme {
	if c.scheme == nil {
		info, err := schemeInfo()
		if err != nil {
			c.scheme = &loginScheme{Name: "native", Data: make(map[string]string)}
		} else {
			c.scheme = info
		}
	}
	return c.scheme
}

func (c *login) Run(context *Context, client *Client) error {
	if c.getScheme().Name == "oauth" {
		return c.oauthLogin(context, client)
	}
	return nativeLogin(context, client)
}

func (c *login) Info() *Info {
	usage := "login [email]"
	return &Info{
		Name:  "login",
		Usage: usage,
		Desc: `Initiates a new tsuru session for a user. If using tsuru native authentication
scheme, it will ask for the email and the password and check if the user is
successfully authenticated. If using OAuth, it will open a web browser for the
user to complete the login.

After that, the token generated by the tsuru server will be stored in
[[${HOME}/.tsuru/token]].

All tsuru actions require the user to be authenticated (except [[tsuru login]]
and [[tsuru version]]).`,
		MinArgs: 0,
	}
}

type logout struct{}

func (c *logout) Info() *Info {
	return &Info{
		Name:  "logout",
		Usage: "logout",
		Desc:  "Logout will terminate the session with the tsuru server.",
	}
}

func (c *logout) Run(context *Context, client *Client) error {
	if url, err := GetURL("/users/tokens"); err == nil {
		request, _ := http.NewRequest("DELETE", url, nil)
		client.Do(request)
	}
	err := filesystem().Remove(JoinWithUserDir(".tsuru", "token"))
	if err != nil && os.IsNotExist(err) {
		return errors.New("You're not logged in!")
	}
	fmt.Fprintln(context.Stdout, "Successfully logged out!")
	return nil
}

type APIRolePermissionData struct {
	Name         string
	ContextType  string
	ContextValue string
}

// APIUser is a user in the tsuru API.
type APIUser struct {
	Email       string
	Roles       []APIRolePermissionData
	Permissions []APIRolePermissionData
}

func (u *APIUser) RoleInstances() []string {
	roles := make([]string, len(u.Roles))
	for i, r := range u.Roles {
		if r.ContextValue != "" {
			r.ContextValue = " " + r.ContextValue
		}
		roles[i] = fmt.Sprintf("%s(%s%s)", r.Name, r.ContextType, r.ContextValue)
	}
	sort.Strings(roles)
	return roles
}

func (u *APIUser) PermissionInstances() []string {
	permissions := make([]string, len(u.Permissions))
	for i, r := range u.Permissions {
		if r.Name == "" {
			r.Name = "*"
		}
		if r.ContextValue != "" {
			r.ContextValue = " " + r.ContextValue
		}
		permissions[i] = fmt.Sprintf("%s(%s%s)", r.Name, r.ContextType, r.ContextValue)
	}
	sort.Strings(permissions)
	return permissions
}

func GetUser(client *Client) (*APIUser, error) {
	url, err := GetURL("/users/info")
	if err != nil {
		return nil, err
	}
	request, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var u APIUser
	err = json.NewDecoder(resp.Body).Decode(&u)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

type userInfo struct{}

func (userInfo) Info() *Info {
	return &Info{
		Name:  "user-info",
		Usage: "user-info",
		Desc:  "Displays information about the current user.",
	}
}

func (userInfo) Run(context *Context, client *Client) error {
	u, err := GetUser(client)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Email: %s\n", u.Email)
	roles := u.RoleInstances()
	if len(roles) > 0 {
		fmt.Fprintf(context.Stdout, "Roles:\n\t%s\n", strings.Join(roles, "\n\t"))
	}
	perms := u.PermissionInstances()
	if len(perms) > 0 {
		fmt.Fprintf(context.Stdout, "Permissions:\n\t%s\n", strings.Join(perms, "\n\t"))
	}
	return nil
}

func PasswordFromReader(reader io.Reader) (string, error) {
	var (
		password []byte
		err      error
	)
	if desc, ok := reader.(descriptable); ok && terminal.IsTerminal(int(desc.Fd())) {
		password, err = terminal.ReadPassword(int(desc.Fd()))
		if err != nil {
			return "", err
		}
	} else {
		fmt.Fscanf(reader, "%s\n", &password)
	}
	if len(password) == 0 {
		msg := "You must provide the password!"
		return "", errors.New(msg)
	}
	return string(password), err
}

func schemeInfo() (*loginScheme, error) {
	url, err := GetURL("/auth/scheme")
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	info := loginScheme{}
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
