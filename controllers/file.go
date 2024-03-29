package controllers

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/thanhpk/randstr"
)

type FileController struct {
	BaseController
}

func (b *FileController) PathPrefix() string {
	return "/files"
}

func (b *FileController) AddRoutes(r gin.IRoutes, prefix string) {
	if prefix == "" {
		prefix = "/"
	}
	r.GET(prefix+"files/:id/meta", b.Meta)
	r.POST(prefix+"files", b.Create)
	r.POST(prefix+"files/:id/upload", b.Upload)
	r.POST(prefix+"files/:id/upload_v2", b.UploadV2)
}

type CreateParams struct {
	FileName  string `json:"file_name" form:"file_name" binding:"required"`
	FileType  string `json:"file_type" form:"file_type" binding:"required"`
	FileSize  int64  `json:"file_size" form:"file_size" binding:"required,numeric"`
	ChunkSize int64  `json:"chunk_size" form:"chunk_size" binding:"required,numeric,min=1024"`
	Prefix    string `json:"prefix" form:"prefix"`
}

type Slice struct {
	Id     string `json:"slice_id"`
	Status int    `json:"status"`
	Sha1   string `json:"sha1"`
}

type FileMeta struct {
	CreateParams
	FileId    string           `json:"file_id" form:"file_id"`
	CreatedAt int64            `json:"created_at" form:"created_at"`
	Status    int              `json:"status" form:"status"`
	Slices    map[string]Slice `json:"slices" form:"slices"`
}

type UploadParams struct {
	FileMeta
	File    *multipart.FileHeader `form:"file" binding:"required"`
	SliceId string                `form:"slice_id" binding:"required,numeric"`
}

func (f *FileController) Meta(c *gin.Context) {
	// get FileId from query
	var meta FileMeta
	var metaFile string
	fileId := c.Param("id")
	cacheDir := path.Join(viper.GetString("uploader.slice_cache_dir"), fileId)

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		// cache not exists, find from uploader
		metaFile = path.Join(viper.GetString("uploader.metafile_dir"), fileId+".meta.json")
	} else {
		// read meta in cache
		metaFile = path.Join(cacheDir, "meta.json")
	}

	if _, err := os.Stat(metaFile); os.IsNotExist(err) {
		logrus.Warningf("meta file not found: %s", metaFile)
		f.Write(c, nil, 404, 0, "")
		return
	}

	content, err := ioutil.ReadFile(metaFile)
	if err != nil {
		logrus.Errorf("failed to read meta file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	json.Unmarshal(content, &meta)
	f.Write(c, meta, 200, 0, "")
}

var filesLock sync.Map

func init() {
	filesLock = sync.Map{}
}

// save all slice to single file
func (f *FileController) UploadV2(c *gin.Context) {
	params := UploadParams{}
	// print all headers with logrus.Debug
	logrus.Debugf("headers: %v", c.Request.Header)
	if err := c.Bind(&params); err != nil {
		logrus.Infof("failed to bind data: %v", err)
		f.Write(c, nil, 400, 0, "")
		return
	}
	sliceDir := path.Join(viper.GetString("uploader.slice_cache_dir"), params.FileId)

	lockAny, _ := filesLock.LoadOrStore(params.FileId, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// check file meta
	var serverFileMeta FileMeta
	content, err := ioutil.ReadFile(path.Join(sliceDir, "meta.json"))
	if err != nil {
		logrus.Errorf("failed to read meta file: %v", err)
		f.Write(c, nil, 422, 0, "")
		return
	}

	json.Unmarshal(content, &serverFileMeta)
	if serverFileMeta.FileName != params.FileName || serverFileMeta.FileType != params.FileType || serverFileMeta.FileSize != params.FileSize {
		logrus.Errorf("meta file is not matched. params %v - servers %v", params, serverFileMeta)
		f.Write(c, nil, 422, 0, "")
		return
	}

	// read file bytes from form
	form, _ := c.MultipartForm()
	file := form.File["file"][0]
	osfile, err := file.Open()
	if err != nil {
		logrus.Errorf("failed to open the uploaded file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	defer osfile.Close()

	fileData, err := ioutil.ReadAll(osfile)
	if err != nil {
		logrus.Errorf("failed to read file: %v", err)
		f.Write(c, nil, 500, 0, "")
	}
	sha1Sum := sha1.Sum(fileData)
	sha1Hex := hex.EncodeToString(sha1Sum[:])

	logrus.Debugf("upload file: %s", file.Filename)

	// open target file
	targetFilePath := path.Join(sliceDir, serverFileMeta.FileName)
	if _, err = os.Stat(targetFilePath); err != nil {
		// create a empty file but with zero bytes filled
		emptyFile, err := os.Create(targetFilePath)
		if err != nil {
			logrus.Errorf("failed to create target file: %v", err)
			f.Write(c, nil, 500, 0, "")
			return
		}
		emptyFile.WriteAt([]byte{0}, serverFileMeta.FileSize-1)
		emptyFile.Close()
	}

	// Open Target File
	targetFile, err := os.OpenFile(targetFilePath, os.O_RDWR, 0644)
	if err != nil {
		logrus.Errorf("failed to open target file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	defer targetFile.Close()

	// write the bytes to target file
	sliceId, _ := strconv.Atoi(params.SliceId)
	offset := params.ChunkSize * int64(sliceId)
	targetFile.WriteAt(fileData, offset)

	// update meta file
	content, _ = os.ReadFile(path.Join(sliceDir, "meta.json"))

	json.Unmarshal(content, &serverFileMeta)

	serverFileMeta.Slices[params.SliceId] = Slice{
		Id:     params.SliceId,
		Status: 1,
		Sha1:   sha1Hex,
	}

	content, _ = json.Marshal(serverFileMeta)
	if err = ioutil.WriteFile(path.Join(sliceDir, "meta.json"), content, 0644); err != nil {
		logrus.Errorf("failed to write meta file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}

	// go over the slices in meta, and check if all slices are uploaded
	for _, slice := range serverFileMeta.Slices {
		if slice.Status != 1 {
			f.Write(c, nil, 206, 0, "")
			return
		}
	}

	// all slices are uploaded, merge them
	filesLock.Delete(params.FileId)
	uploadDir := viper.GetString("uploader.upload_dir")
	if serverFileMeta.Prefix != "" {
		uploadDir = path.Join(uploadDir, serverFileMeta.Prefix)
	}
	os.MkdirAll(uploadDir, 0755)

	// move target file to upload dir
	os.Rename(targetFilePath, path.Join(uploadDir, serverFileMeta.FileName))

	// 这里保留 meta 文件不删除
	// ...

	f.Write(c, nil, 200, 0, "")
}

func (f *FileController) Upload(c *gin.Context) {
	params := UploadParams{}
	// print all headers with logrus.Debug
	logrus.Debugf("headers: %v", c.Request.Header)

	if err := c.Bind(&params); err != nil {
		logrus.Infof("failed to bind data: %v", err)
		f.Write(c, nil, 400, 0, "")
		return
	}

	sliceDir := path.Join(viper.GetString("uploader.slice_cache_dir"), params.FileId)

	// update meta file, should be atomic
	lockAny, _ := filesLock.LoadOrStore(params.FileId, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// check file meta
	var serverFileMeta FileMeta
	content, err := ioutil.ReadFile(path.Join(sliceDir, "meta.json"))
	if err != nil {
		logrus.Errorf("failed to read meta file: %v", err)
		f.Write(c, nil, 422, 0, "")
		return
	}

	json.Unmarshal(content, &serverFileMeta)
	if serverFileMeta.FileName != params.FileName || serverFileMeta.FileType != params.FileType || serverFileMeta.FileSize != params.FileSize {
		logrus.Errorf("meta file is not matched. params %v - servers %v", params, serverFileMeta)
		f.Write(c, nil, 422, 0, "")
		return
	}

	form, _ := c.MultipartForm()
	file := form.File["file"][0]
	osfile, err := file.Open()
	if err != nil {
		logrus.Errorf("failed to open the uploaded file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	defer osfile.Close()

	fileData, err := ioutil.ReadAll(osfile)
	if err != nil {
		logrus.Errorf("failed to read file: %v", err)
		f.Write(c, nil, 500, 0, "")
	}
	sha1Sum := sha1.Sum(fileData)
	sha1Hex := hex.EncodeToString(sha1Sum[:])

	logrus.Debugf("upload file: %s", file.Filename)
	fileSlicePath := path.Join(sliceDir, serverFileMeta.FileName+"."+params.SliceId+"."+sha1Hex+".slice")
	if err = c.SaveUploadedFile(file, fileSlicePath); err != nil {
		logrus.Errorf("failed to save file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}

	content, _ = os.ReadFile(path.Join(sliceDir, "meta.json"))

	json.Unmarshal(content, &serverFileMeta)

	serverFileMeta.Slices[params.SliceId] = Slice{
		Id:     params.SliceId,
		Status: 1,
		Sha1:   sha1Hex,
	}

	content, _ = json.Marshal(serverFileMeta)
	if err = ioutil.WriteFile(path.Join(sliceDir, "meta.json"), content, 0644); err != nil {
		logrus.Errorf("failed to write meta file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}

	// go over the slices in meta, and check if all slices are uploaded
	for _, slice := range serverFileMeta.Slices {
		if slice.Status != 1 {
			f.Write(c, nil, 206, 0, "")
			return
		}
	}

	// all slices are uploaded, merge them
	filesLock.Delete(params.FileId)
	uploadDir := viper.GetString("uploader.upload_dir")
	if serverFileMeta.Prefix != "" {
		uploadDir = path.Join(uploadDir, serverFileMeta.Prefix)
	}
	os.MkdirAll(uploadDir, 0755)
	destFile, err := os.OpenFile(path.Join(uploadDir, serverFileMeta.FileName), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		logrus.Errorf("failed to create dest file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	defer destFile.Close()
	metaFilePath := path.Join(viper.GetString("uploader.metafile_dir"), params.FileId+".meta.json")
	destMetaFile, err := os.Create(metaFilePath)
	if err != nil {
		logrus.Errorf("failed to create dest meta file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}
	defer destMetaFile.Close()

	io.Copy(destMetaFile, bytes.NewReader(content))

	for i := 0; i < len(serverFileMeta.Slices); i++ {
		slice := serverFileMeta.Slices[strconv.Itoa(i)]
		sliceFilePath := path.Join(sliceDir, serverFileMeta.FileName+"."+slice.Id+"."+slice.Sha1+".slice")
		sliceFile, err := os.Open(sliceFilePath)
		if err != nil {
			logrus.Errorf("failed to open slice file: %v", err)
			f.Write(c, nil, 500, 0, "")
			return
		}
		io.Copy(destFile, sliceFile)
		sliceFile.Close()
	}

	// remove slice dir
	os.RemoveAll(sliceDir)

	// return 200
	f.Write(c, nil, 200, 0, "")
}

func (f *FileController) Create(c *gin.Context) {
	// send some information to prepare the multi-part uploading
	// information of file includes:
	// file_name
	// file_type
	// file_size, in bytes
	// chunk_size, in bytes, default 10 * 1024 ** 2
	//
	// server will create a temp dir somewhere to receive the file slices
	params := CreateParams{}
	if err := c.BindJSON(&params); err != nil {
		logrus.Infof("failed to bind json: %v", err)
		f.Write(c, nil, 400, 0, "")
		return
	}

	if strings.Contains(params.Prefix, "..") {
		f.Write(c, nil, 400, 0, "")
		return
	}

	var fileId string
	var cacheDirPath string
	for i := 0; i < 10; i++ {
		fileId = randstr.Hex(32)
		// join config and fileId as dir
		cacheDirPath = path.Join(viper.GetString("uploader.slice_cache_dir"), fileId)
		if _, err := os.Stat(cacheDirPath); err != nil {
			if err == nil {
				continue
			}
			os.MkdirAll(cacheDirPath, os.ModePerm)
			break
		}
	}

	meta := FileMeta{
		CreateParams: params,
		FileId:       fileId,
		CreatedAt:    time.Now().Unix(),
		Status:       0,
		Slices:       make(map[string]Slice),
	}

	var sliceNum int64
	if params.FileSize%params.ChunkSize != 0 {
		sliceNum = params.FileSize/params.ChunkSize + 1
	} else {
		sliceNum = params.FileSize / params.ChunkSize
	}

	for i := int64(0); i < sliceNum; i++ {
		sliceId := strconv.FormatInt(i, 10)
		slice := Slice{
			Id:     sliceId,
			Status: 0,
			Sha1:   "",
		}
		meta.Slices[sliceId] = slice
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		logrus.Errorf("failed to marshal meta data: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}

	metaFilePath := path.Join(cacheDirPath, "meta.json")
	if err := ioutil.WriteFile(metaFilePath, metaData, 0644); err != nil {
		logrus.Errorf("failed to write meta data to file: %v", err)
		f.Write(c, nil, 500, 0, "")
		return
	}

	f.Write(c, meta, 200, 0, "")
}
