package main

import (
	"errors"
	"fmt"
	"log"
	"runtime"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nkeys"
)

// Warning! You will want this in a secret store
// of some kind instead of the source code like this
const NKeySeed string = "SAAIRUPUPZ4CZZX4EYX2MF6A3KN7FGI3AQPEVF3HI2JXVNV6DJGSTZGDFE"

var issuerKeypair nkeys.KeyPair

func main() {
	err := RunAuthService()
	if err != nil {
		log.Fatal(err)
	}
}

func RunAuthService() error {
	nc, err := nats.Connect(nats.DefaultURL, nats.UserInfo("auth", "auth"))
	if err != nil {
		return err
	}
	defer nc.Drain()

	issuerKeypair, err = nkeys.FromSeed([]byte(NKeySeed))
	if err != nil {
		return err
	}

	_, err = micro.AddService(nc, micro.Config{
		Name:        "auth",
		Version:     "0.0.1",
		Description: "Handle authorization of Google JWTs for chat applications",
		Endpoint: &micro.EndpointConfig{
			Subject: "$SYS.REQ.USER.AUTH",
			Handler: micro.HandlerFunc(AuthHandler),
		},
	})
	if err != nil {
		return err
	}

	runtime.Goexit()
	return nil
}

func AuthHandler(r micro.Request) {
	fmt.Println("Received Request")

	rc, err := jwt.DecodeAuthorizationRequestClaims(string(r.Data()))
	if err != nil {
		fmt.Println("Error", err)
		r.Error("500", err.Error(), nil)
	}

	userNkey := rc.UserNkey
	serverId := rc.Server.ID
	claims := jwt.NewUserClaims(rc.UserNkey)

	// this gives me a backdoor with the CLI. Don't do this in production!
	if rc.ConnectOptions.Username == "user" && rc.ConnectOptions.Password == "my-password" {
		claims.Name = rc.ConnectOptions.Username
		// uc.Audience =
		claims.Permissions = jwt.Permissions{}

		token, err := ValidateAndSign(claims, issuerKeypair)
		Respond(r, userNkey, serverId, token, err.Error())
		return
	}

	r.Error("500", "Not implemented", nil)
}

func Respond(req micro.Request, userNKey, serverId, userJWT, errMsg string) {
	rc := jwt.NewAuthorizationResponseClaims(userNKey)
	rc.Audience = serverId
	rc.Error = errMsg
	rc.Jwt = userJWT

	token, err := rc.Encode(issuerKeypair)
	if err != nil {
		log.Println("error encoding response jwt:", err)
	}

	req.Respond([]byte(token))
}

func ValidateAndSign(claims *jwt.UserClaims, kp nkeys.KeyPair) (string, error) {
	// Validate the claims.
	vr := jwt.CreateValidationResults()
	claims.Validate(vr)
	if len(vr.Errors()) > 0 {
		return "", errors.Join(vr.Errors()...)
	}

	// Sign it with the issuer key since this is non-operator mode.
	return claims.Encode(kp)
}
