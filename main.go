package main

import (
	"context"
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/jakeslee/aliyundrive"
	aliWebdav "github.com/jakeslee/aliyundrive-webdav/internal/webdav"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/webdav"
	"log"
	"net/http"
)

func main() {
	refreshToken := "***REMOVED***"

	logrus.SetFormatter(&nested.Formatter{
		HideKeys: true,
	})

	drive := aliyundrive.NewClient(&aliyundrive.Options{
		AutoRefresh: true,
	})

	cred, err := drive.AddCredential(aliyundrive.NewCredential(&aliyundrive.Credential{
		RefreshToken: refreshToken,
	}).RegisterChangeEvent(func(credential *aliyundrive.Credential) {
		logrus.Infof("credential changed")
	}))

	if err != nil {
		logrus.Errorf("add credential error %s", err)
		return
	}

	h := &webdav.Handler{
		FileSystem: aliWebdav.NewAliDriveFS(drive, cred),
		LockSystem: webdav.NewMemLS(),
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		logrus.Infof("request %s %s", request.Method, request.RequestURI)

		ctx := context.WithValue(request.Context(), aliWebdav.CtxSizeValue, request.ContentLength)

		ctxRequest := request.WithContext(ctx)

		h.ServeHTTP(writer, ctxRequest)
	})

	logrus.Infof("webdav server started at %s")
	log.Fatal(http.ListenAndServe(":18080", nil))
}
