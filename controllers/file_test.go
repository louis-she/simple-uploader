package controllers_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/louis-she/simple-uploader/controllers"
	"github.com/louis-she/simple-uploader/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

var r *gin.Engine

func TestMain(m *testing.M) {
	os.Setenv("GIN_MODE", "test")
	logrus.SetLevel(logrus.DebugLevel)
	viper.SetDefault("uploader.slice_cache_dir", "/tmp/golang_test_dev/cache")
	viper.SetDefault("uploader.upload_dir", "/tmp/golang_test_dev/data")

	os.MkdirAll(viper.GetString("uploader.slice_cache_dir"), 0755)
	os.MkdirAll(viper.GetString("uploader.upload_dir"), 0755)

	r = gin.New()
	controllers.Attach(r, "/")

	m.Run()
	// remove all temp files
	logrus.Debug("remove test directory")
	os.RemoveAll("/tmp/golang_test_dev")
	os.Exit(0)
}

func prepareContext(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	c := gin.CreateTestContextOnly(w, r)
	c.Request = req
	return c, w
}

func createFileWithRequest(req *http.Request) *httptest.ResponseRecorder {
	c, w := prepareContext(req)
	b := controllers.FileController{}
	b.Create(c)
	return w
}

func generateRandomLargeFile(fileSize int64) (file *os.File) {
	// write a random 10MB temp file to disk
	if fileSize == 0 {
		fileSize = 1024 * 1024 * 10
	}
	token := make([]byte, fileSize)
	rand.Read(token)
	// create a temp file
	file, _ = os.CreateTemp("", "test")
	file.Write(token)
	file.Seek(0, 0)
	return
}

func createRandomFile(fileSize int64, chunkSize int64) (*os.File, controllers.FileMeta) {
	if chunkSize == 0 {
		chunkSize = 3 * 1024 * 1024
	}
	file := generateRandomLargeFile(fileSize)
	fileStat, _ := os.Stat(file.Name())
	params := controllers.CreateParams{
		FileName:  filepath.Base(file.Name()),
		FileType:  "text/plain",
		FileSize:  fileStat.Size(),
		ChunkSize: chunkSize, // smaller than file size
	}

	// upload the first chunk
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "/files", bytes.NewBuffer(body))
	w := createFileWithRequest(req)

	var response controllers.Response
	var responseMeta controllers.FileMeta
	content, _ := io.ReadAll(w.Body)
	json.Unmarshal(content, &response)
	json.Unmarshal(response.Data, &responseMeta)

	return file, responseMeta
}

func uploadSlice(slice int64, meta controllers.FileMeta, file *os.File, assert *assert.Assertions) *httptest.ResponseRecorder {
	multipartBody := &bytes.Buffer{}
	writer := multipart.NewWriter(multipartBody)
	writer.WriteField("file_id", meta.FileId)
	writer.WriteField("chunk_size", strconv.FormatInt(meta.ChunkSize, 10))
	writer.WriteField("file_type", meta.FileType)
	writer.WriteField("file_name", meta.FileName)
	writer.WriteField("file_size", strconv.FormatInt(meta.FileSize, 10))
	writer.WriteField("slice_id", strconv.FormatInt(slice, 10))
	writer.WriteField("created_at", strconv.FormatInt(meta.CreatedAt, 10))
	writer.WriteField("status", strconv.Itoa(meta.Status))

	fileWriter, _ := writer.CreateFormFile("file", file.Name())
	sliceChunkSize := utils.Min(meta.FileSize-int64(slice)*meta.ChunkSize, meta.ChunkSize)

	buf := make([]byte, sliceChunkSize)
	fileReader, _ := os.Open(file.Name())
	offset := slice * meta.ChunkSize
	fileReader.Seek(offset, 0)
	io.ReadFull(fileReader, buf)
	io.Copy(fileWriter, bytes.NewReader(buf))
	writer.Close()
	req, _ := http.NewRequest("POST", "/files/"+meta.FileId+"/upload", multipartBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c, w := prepareContext(req)
	r.HandleContext(c)
	assert.True(w.Code == http.StatusOK || w.Code == http.StatusPartialContent)

	return w
}

func TestCreateFileNoArgs(t *testing.T) {
	assert := assert.New(t)
	req, _ := http.NewRequest("POST", "/files", nil)
	w := createFileWithRequest(req)
	assert.Equal(http.StatusBadRequest, w.Code)
}

func TestCreateFileInvalidArgs(t *testing.T) {
	assert := assert.New(t)

	params := controllers.CreateParams{
		FileName:  "test.txt",
		FileType:  "text/plain",
		FileSize:  1024,
		ChunkSize: 10,
	}

	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "/files", bytes.NewBuffer(body))
	w := createFileWithRequest(req)
	assert.Equal(http.StatusBadRequest, w.Code)
}

func TestCreateFileValid(t *testing.T) {
	assert := assert.New(t)
	file := generateRandomLargeFile(0)
	defer os.Remove(file.Name())

	params := controllers.CreateParams{
		FileName:  "test.txt",
		FileType:  "text/plain",
		FileSize:  1024 * 1024 * 10,
		ChunkSize: 1024 * 10,
	}
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "/files", bytes.NewBuffer(body))
	w := createFileWithRequest(req)
	assert.Equal(http.StatusOK, w.Code)

	// get json from w body
	var response controllers.Response
	var responseFileMeta controllers.FileMeta
	content, _ := io.ReadAll(w.Body)
	json.Unmarshal(content, &response)
	json.Unmarshal(response.Data, &responseFileMeta)

	assert.Equal(params.FileName, responseFileMeta.FileName)
	assert.Equal(params.FileType, responseFileMeta.FileType)
	assert.Equal(params.FileSize, responseFileMeta.FileSize)
	assert.Equal(params.ChunkSize, responseFileMeta.ChunkSize)
	assert.Equal(responseFileMeta.Status, 0)
	assert.Less(time.Now().Unix()-responseFileMeta.CreatedAt, int64(4))
	assert.GreaterOrEqual(time.Now().Unix()-responseFileMeta.CreatedAt, int64(0))
	assert.NotEmpty(responseFileMeta.FileId)

	// read content from file
	var fileMeta controllers.FileMeta
	metaFile := path.Join(viper.GetString("uploader.slice_cache_dir"), responseFileMeta.FileId, "meta.json")
	content, _ = os.ReadFile(metaFile)
	json.Unmarshal(content, &fileMeta)
	assert.Equal(params.FileName, fileMeta.FileName)
	assert.Equal(params.FileType, fileMeta.FileType)
	assert.Equal(params.FileSize, fileMeta.FileSize)
	assert.Equal(params.ChunkSize, fileMeta.ChunkSize)
	assert.Equal(responseFileMeta.FileId, fileMeta.FileId)
	assert.Equal(responseFileMeta.Status, fileMeta.Status)
	assert.Equal(responseFileMeta.CreatedAt, fileMeta.CreatedAt)
	assert.Equal(len(fileMeta.Slices), 1024)
}

func TestCreateFileValidWithExtraSlice(t *testing.T) {
	assert := assert.New(t)
	file, responseFileMeta := createRandomFile(1024*1024*10+100, 1024*10)
	defer os.Remove(file.Name())
	var fileMeta controllers.FileMeta
	metaFile := path.Join(viper.GetString("uploader.slice_cache_dir"), responseFileMeta.FileId, "meta.json")
	content, _ := os.ReadFile(metaFile)
	json.Unmarshal(content, &fileMeta)
	assert.Equal(len(fileMeta.Slices), 1025)
}

func TestFileUploadSingle(t *testing.T) {
	assert := assert.New(t)
	file, responseMeta := createRandomFile(0, 10*1024*1024)
	defer os.Remove(file.Name())

	// upload
	w := uploadSlice(0, responseMeta, file, assert)
	assert.Equal(http.StatusOK, w.Code)

	// compare uploaded sha1
	fileContent := make([]byte, responseMeta.FileSize)
	file.Seek(0, 0)
	file.Read(fileContent)
	sha1Sum := sha1.Sum(fileContent)
	sha1Hex := hex.EncodeToString(sha1Sum[:])

	destFilePath := path.Join(viper.GetString("uploader.upload_dir"), responseMeta.FileName)
	serverFileContent, _ := os.ReadFile(destFilePath)
	sha1Sum = sha1.Sum(serverFileContent)
	sha1HexServer := hex.EncodeToString(sha1Sum[:])
	assert.Equal(sha1Hex, sha1HexServer)
}

func TestFileUploadWithPrefix(t *testing.T) {
	assert := assert.New(t)
	file := generateRandomLargeFile(0)
	fileStat, _ := os.Stat(file.Name())
	params := controllers.CreateParams{
		FileName:  filepath.Base(file.Name()),
		FileType:  "text/plain",
		FileSize:  fileStat.Size(),
		ChunkSize: 1024 * 1024 * 10,
		Prefix:    "test_prefix",
	}

	// upload the first chunk
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "/files", bytes.NewBuffer(body))
	c, w := prepareContext(req)
	r.HandleContext(c)

	var response controllers.Response
	var responseMeta controllers.FileMeta
	content, _ := io.ReadAll(w.Body)
	json.Unmarshal(content, &response)
	json.Unmarshal(response.Data, &responseMeta)

	assert.Equal(http.StatusOK, w.Code)

	uploadSlice(0, responseMeta, file, assert)
	assert.FileExists(path.Join(viper.GetString("uploader.upload_dir"), "test_prefix", responseMeta.FileName))
}

func TestFildUploadMultipleSlices(t *testing.T) {
	assert := assert.New(t)
	file, responseMeta := createRandomFile(0, 0)
	defer os.Remove(file.Name())

	fileReader, _ := os.Open(file.Name())
	slicesNum := int(responseMeta.FileSize/responseMeta.ChunkSize) + 1
	slicesDir := path.Join(viper.GetString("uploader.slice_cache_dir"), responseMeta.FileId)
	for i := 0; i < slicesNum; i++ {
		// upload
		w := uploadSlice(int64(i), responseMeta, fileReader, assert)

		if i != slicesNum-1 {
			assert.Equal(http.StatusPartialContent, w.Code)
			// cache slice dir should has a file named: "fileName.sliceId.sha1hex"
			fileContent := make([]byte, responseMeta.ChunkSize)
			file.Seek(int64(i)*responseMeta.ChunkSize, 0)
			file.Read(fileContent)
			sha1Sum := sha1.Sum(fileContent)
			sha1Hex := hex.EncodeToString(sha1Sum[:])
			cacheSlicePath := path.Join(slicesDir, responseMeta.FileName+"."+strconv.Itoa(i)+"."+sha1Hex+".slice")
			assert.FileExists(cacheSlicePath)
		} else {
			destFilePath := path.Join(viper.GetString("uploader.upload_dir"), responseMeta.FileName)
			assert.Equal(http.StatusOK, w.Code)
			assert.NoDirExists(slicesDir)
			assert.FileExists(destFilePath)
			// compare sha1
			localBytes := make([]byte, responseMeta.FileSize)
			file.Seek(0, 0)
			file.Read(localBytes)
			localSha1Sum := sha1.Sum(localBytes)
			localSha1Hex := hex.EncodeToString(localSha1Sum[:])

			serverBytes, _ := os.ReadFile(destFilePath)
			serverSha1Sum := sha1.Sum(serverBytes)
			serverSha1Hex := hex.EncodeToString(serverSha1Sum[:])
			assert.Equal(localSha1Hex, serverSha1Hex)
		}
	}

	logrus.Debug("OK")
}

func TestFileUploadInteruptResume(t *testing.T) {
	assert := assert.New(t)
	file, responseMeta := createRandomFile(0, 0)
	defer os.Remove(file.Name())

	uploadSlice(0, responseMeta, file, assert)
	uploadSlice(2, responseMeta, file, assert)

	// get meta data
	req, _ := http.NewRequest("GET", "/files/"+responseMeta.FileId+"/meta", nil)
	c, w := prepareContext(req)
	r.HandleContext(c)
	assert.Equal(http.StatusOK, w.Code)

	var meta controllers.FileMeta
	var response controllers.Response
	json.Unmarshal(w.Body.Bytes(), &response)
	json.Unmarshal(response.Data, &meta)

	downloadedSliceNum := 0
	for _, slice := range meta.Slices {
		if slice.Status == 1 {
			downloadedSliceNum++
		}
	}

	assert.Equal(downloadedSliceNum, 2)

	// continue upload the rest depending on slice(slice 1 and 3)
	for _, slice := range meta.Slices {
		if slice.Status == 1 {
			continue
		}
		sliceId, _ := strconv.Atoi(slice.Id)
		uploadSlice(int64(sliceId), responseMeta, file, assert)
	}

	// all file uploaded
	assert.NoDirExists(path.Join(viper.GetString("uploader.slice_cache_dir"), responseMeta.FileId))
	destFilePath := path.Join(viper.GetString("uploader.upload_dir"), responseMeta.FileName)
	assert.FileExists(destFilePath)

	// compare uploaded sha1
	localBytes := make([]byte, responseMeta.FileSize)
	file.Seek(0, 0)
	file.Read(localBytes)
	localSha1Sum := sha1.Sum(localBytes)
	localSha1Hex := hex.EncodeToString(localSha1Sum[:])

	serverBytes, _ := os.ReadFile(destFilePath)
	serverSha1Sum := sha1.Sum(serverBytes)
	serverSha1Hex := hex.EncodeToString(serverSha1Sum[:])
	assert.Equal(localSha1Hex, serverSha1Hex)
}
