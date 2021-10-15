package main

import (
	"context"
	"fmt"
	"github.com/alexflint/go-arg"
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/jakeslee/aliyundrive"
	"github.com/jakeslee/aliyundrive-webdav/internal"
	aliWebdav "github.com/jakeslee/aliyundrive-webdav/internal/webdav"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/webdav"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
)

const defaultRefreshTokenFile = "refresh_token"

func main() {
	arg.MustParse(internal.Config)

	logrus.SetFormatter(&nested.Formatter{
		HideKeys: true,
	})

	logrus.Infof("aliyundrive-webdav v%s", internal.Version)

	drive := aliyundrive.NewClient(&aliyundrive.Options{
		AutoRefresh: true,
	})

	rtFromFile := internal.Config.RefreshToken

	filePath := filepath.Join(internal.Config.WorkDir, defaultRefreshTokenFile)
	fileBytes, err := ioutil.ReadFile(filePath)
	if err == nil {
		rtFromFile = string(fileBytes)
	}

	cred, err := drive.AddCredential(aliyundrive.NewCredential(&aliyundrive.Credential{
		RefreshToken: rtFromFile,
	}).RegisterChangeEvent(func(credential *aliyundrive.Credential) {
		logrus.Infof("backend aliyundrive user[%s@%s] is launched! credential loaded!", credential.Name, credential.UserId)

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			logrus.Warnf("write refresh token cache file error, %s", err)
			return
		}
		defer file.Close()

		_, err = file.Write([]byte(credential.RefreshToken))
		if err != nil {
			logrus.Warnf("write refresh token cache file error, %s", err)
			return
		}
	}))

	if err != nil {
		logrus.Errorf("add credential error %s", err)
		return
	}

	h := &aliWebdav.Handler{
		Handler: webdav.Handler{
			FileSystem: aliWebdav.NewAliDriveFS(drive, cred, internal.Config.RapidUpload),
			LockSystem: webdav.NewMemLS(),
		},
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		logrus.Infof("request %s %s", request.Method, request.RequestURI)

		if requestSuffersFinderProblem(request) {
			err := handleFinderRequest(writer, request)
			if err != nil {
				return
			}
		}

		ctx := context.WithValue(request.Context(), aliWebdav.CtxSizeValue, request.ContentLength)

		ctxRequest := request.WithContext(ctx)

		h.ServeHTTP(writer, ctxRequest)
	})

	hosted := fmt.Sprintf("%s:%d", internal.Config.Host, internal.Config.Port)

	logrus.Infof("webdav server started at %s", hosted)
	log.Fatal(http.ListenAndServe(hosted, nil))
}

func requestSuffersFinderProblem(r *http.Request) bool {
	return r.Header.Get("X-Expected-Entity-Length") != ""
}

func handleFinderRequest(w http.ResponseWriter, r *http.Request) error {
	logrus.Warnf("finder problem intercepted, content-length %s, x-expected-entity-length %s",
		r.Header.Get("Content-Length"), r.Header.Get("X-Expected-Entity-Length"))

	expected := r.Header.Get("X-Expected-Entity-Length")
	expectedInt, err := strconv.ParseInt(expected, 10, 64)
	if err != nil {
		logrus.Errorf("error %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return err
	}
	r.ContentLength = expectedInt
	return nil
}
