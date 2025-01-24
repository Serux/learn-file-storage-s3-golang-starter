package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

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
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error3", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User video not valid", nil)
		return
	}

	const uploadmax int64 = 1 << 30

	r.ParseMultipartForm(uploadmax)
	file, fileheader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error1", err)
		return
	}
	defer file.Close()

	mimetype, _, err := mime.ParseMediaType(fileheader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error4", err)
		return
	}
	if mimetype != "video/mp4" {
		respondWithError(w, http.StatusUnauthorized, "mime wrong", nil)
		return
	}

	fil, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "error3", err)
		return
	}
	defer os.Remove(fil.Name())
	defer fil.Close()
	fmt.Println(fil.Name())

	_, err = io.Copy(fil, file)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Error coping", err)
		return
	}

	_, err = fil.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Error seeeking", err)
		return
	}
	processed, _ := processVideoForFastStart(fil.Name())
	fmt.Println(processed)
	profil, _ := os.Open(processed)
	defer os.Remove(processed)
	defer profil.Close()

	ratio, _ := getVideoAspectRatio(fil.Name())

	ratioString := ""
	if ratio == "9:16" {
		ratioString = "landscape"
	} else if ratio == "16:9" {
		ratioString = "portrait"
	} else {
		ratioString = "other"
	}
	fmt.Println(ratioString)

	name := make([]byte, 32)
	rand.Read(name)
	vidid := ratioString + "/" + base64.RawURLEncoding.EncodeToString(name) + "." + strings.Split(mimetype, "/")[1]

	fullbucket := fmt.Sprintf("%v", cfg.s3Bucket)
	//fullbucket := fmt.Sprintf("arn:aws:s3:%v::%v", cfg.s3Region, cfg.s3Bucket)

	fmt.Println(vidid)

	_, err = cfg.s3Client.PutObject(
		context.Background(),
		&s3.PutObjectInput{
			Bucket:      &fullbucket,
			ContentType: &mimetype,
			Key:         &vidid,
			Body:        profil,
		},
	)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Error Uploading", err)
		return
	}
	//url := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, vidid)
	url := fmt.Sprintf("%v,%v", cfg.s3Bucket, vidid)
	video.VideoURL = &url

	cfg.db.UpdateVideo(video)
	newv, _ := cfg.dbVideoToSignedVideo(video)
	respondWithJSON(w, http.StatusOK, newv)

}

func getVideoAspectRatio(filePath string) (string, error) {
	c := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buff := bytes.Buffer{}
	c.Stdout = &buff
	type outJson struct {
		Streams []struct {
			Height int
			Width  int
		}
	}
	res := outJson{}
	c.Run()
	json.Unmarshal(buff.Bytes(), &res)
	if res.Streams[0].Height/res.Streams[0].Width == 16/9 {
		return "16:9", nil
	}
	if res.Streams[0].Height/res.Streams[0].Width == 9/16 {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	fileOut := filePath + ".processing"
	c := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", fileOut)
	c.Run()
	return fileOut, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	precli := s3.NewPresignClient(s3Client)
	rq, _ := precli.PresignGetObject(context.TODO(), &s3.GetObjectInput{Bucket: &bucket, Key: &key}, s3.WithPresignExpires(expireTime))
	return rq.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	videoUrl := strings.Split(*video.VideoURL, ",")
	fmt.Println(*video.VideoURL)
	signedUrl, _ := generatePresignedURL(cfg.s3Client, videoUrl[0], videoUrl[1], time.Minute*1)
	video.VideoURL = &signedUrl
	fmt.Println(*video.VideoURL)
	return video, nil

}
