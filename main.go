package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

const maxUploadSize = 1e+7 // 10 MB
const uploadPath = "www/data"
const dataPath = "www/data"

//File ...
type File struct {
	Name string
	Path string
	Size int64
}

//IndexViewModel ...
type IndexViewModel struct {
	Name  string
	Time  string
	Files []File
}

//FileViewModel ...
type FileViewModel struct {
	Name    string
	File    File
	Data    CsvFileData
	Message string
}

//CsvFileData ...
type CsvFileData struct {
	Columns []string
	Values  [][]string
}

//PreviewContent ...
func (c File) PreviewContent() string {
	// This cast is O(N)
	runes := bytes.Runes([]byte(c.Name))
	if len(runes) > 30 {
		return string(runes[:30]) + "..."
	}
	return string(runes)
}

func getDataPath() string {
	pwd, _ := os.Getwd()
	path := filepath.Join(pwd, dataPath)
	fmt.Println(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.MkdirAll(path, os.ModePerm)
	}

	return path
}

// ReadCsvFile ..
func ReadCsvFile(name string) CsvFileData {
	var columnNames []string
	var rows [][]string
	path := filepath.Join(getDataPath(), name)
	csvFile, _ := os.Open(path)
	reader := csv.NewReader(bufio.NewReader(csvFile))
	i := 0
	for {
		line, error := reader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			log.Fatal(error)
		}

		if i == 0 {
			for j := 0; j < len(line); j++ {
				columnNames = append(columnNames, line[j])
			}
		} else {
			var row []string
			for j := 0; j < len(line); j++ {
				row = append(row, line[j])
			}
			rows = append(rows, row)
		}
		i++
	}

	// return CsvFileData{
	// 	Columns: []string{"name", "val"},
	// 	Values:  [][]string{{"test1", "1"}, {"test2", "2"}},
	// }

	return CsvFileData{
		Columns: columnNames,
		Values:  rows,
	}
}

// GetFiles ...
func GetFiles() []File {
	var files []File
	var fileInfos []os.FileInfo

	fileInfos, err := ioutil.ReadDir(getDataPath())
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range fileInfos {
		fmt.Println(file.Name())
		files = append(files, File{
			Name: file.Name(),
			Size: file.Size(),
		})
	}

	if err != nil {
		return nil
	}

	return files
}

func renderError(w http.ResponseWriter, errorCode string, httpCode int) {
	http.Error(w, errorCode, httpCode)
}

// IndexHandler ...
func IndexHandler(w http.ResponseWriter, r *http.Request) {

	files := GetFiles()

	templates := template.Must(template.ParseFiles("www/templates/index.html"))
	indexData := IndexViewModel{
		Name:  "Anonymous",
		Time:  time.Now().Format(time.Stamp),
		Files: files}

	//Takes the name from the URL query e.g ?name=Martin, will set indexData.Name = Martin.
	if name := r.FormValue("name"); name != "" {
		indexData.Name = name
	}
	if err := templates.ExecuteTemplate(w, "index.html", indexData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// FileHandler ...
func FileHandler(w http.ResponseWriter, r *http.Request) {

	templates := template.Must(template.ParseFiles("www/templates/file.html"))

	fileViewModel := FileViewModel{}

	//Takes the name from the URL query e.g ?name=Martin, will set welcome.Name = Martin.
	if name := r.FormValue("name"); name != "" {
		fileViewModel.Name = name
		fileViewModel.Data = ReadCsvFile(name)
	} else {
		fileViewModel.Message = "Missin parameter: name"
	}

	//If errors show an internal server error message
	//I also pass the welcome struct to the welcome-template.html file.
	if err := templates.ExecuteTemplate(w, "file.html", fileViewModel); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// UploadFileHandler ...
func UploadFileHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

		if r.Method != "POST" {
			http.Redirect(w, r, "/", 301)
		}

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			renderError(w, "FILE_TOO_BIG", http.StatusBadRequest)
			return
		}

		fileType := r.PostFormValue("type")
		file, header, err := r.FormFile("file")

		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)

		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}

		filetype := http.DetectContentType(fileBytes)
		if filetype != "text/plain; charset=utf-8" {
			renderError(w, "INVALID_FILE_TYPE", http.StatusBadRequest)
			return
		}

		fileName := header.Filename
		// fileEndings, err := mime.ExtensionsByType(fileType)
		// if err != nil {
		// 	renderError(w, "CANT_READ_FILE_TYPE", http.StatusInternalServerError)
		// 	return
		// }

		newPath := filepath.Join(uploadPath, fileName)
		fmt.Printf("FileType: %s, File: %s\n", fileType, newPath)

		newFile, err := os.Create(newPath)
		if err != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", 301)
		// w.Write([]byte("SUCCESS"))
	})
}

func main() {

	http.Handle("/static/", //final url can be anything
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("www/static"))))

	fs := http.FileServer(http.Dir(uploadPath))
	http.Handle("/files/", http.StripPrefix("/files", fs))
	http.HandleFunc("/", IndexHandler)
	http.HandleFunc("/file", FileHandler)
	http.HandleFunc("/upload", UploadFileHandler())

	err := http.ListenAndServe("localhost:8080", nil)

	if err != nil {
		log.Fatal("ListenAndServe: ", err.Error())
	}
}
