package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory int64 = 10 << 20

	r.ParseMultipartForm(maxMemory)

	file, fileheader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error1", err)
		return
	}
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error3", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User video not valid", nil)
		return
	}
	mimetype, _, err := mime.ParseMediaType(fileheader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "mime wrong", err)
		return
	}
	defer file.Close()

	fmt.Println(mimetype)

	if mimetype != "image/png" && mimetype != "image/jpeg" {
		respondWithError(w, http.StatusUnauthorized, "mime wrong", nil)
		return
	}
	name := make([]byte, 32)
	rand.Read(name)
	thuid := base64.RawURLEncoding.EncodeToString(name) + "." + strings.Split(mimetype, "/")[1]
	fp := filepath.Join(cfg.assetsRoot, thuid)
	fileW, err := os.Create(fp)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "ERR", err)
		return
	}
	//TODO handle copy err
	io.Copy(fileW, file)

	url := fmt.Sprintf("http://localhost:%v/assets/%v", cfg.port, thuid)
	video.ThumbnailURL = &url

	cfg.db.UpdateVideo(video)

	respondWithJSON(w, http.StatusOK, video)
}
