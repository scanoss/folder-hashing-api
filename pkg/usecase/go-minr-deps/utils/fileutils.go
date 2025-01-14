package utils

import (
	"bufio"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"hash/crc64"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	m "scanoss.com/hfh-api/pkg/usecase/go-minr-deps/model"
)

type FileItem struct {
	Path  string
	Name  string
	Size  uint64
	Score float64
	Key   string
}

type FileDesc struct {
	Name      string
	IsSymlink bool
}
type DownloadItem struct {
	TempPath string
	Content  string
	Size     int64
	KeyName  string
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func CropVeryLongLines(refFile string, input []byte, maxLen int, logEnabled bool) []byte {
	lines := strings.Split(string(input), "\n")
	for i, line := range lines {
		if len(line) > maxLen {
			lines[i] = "\n"
			if logEnabled {
				log.Printf("Line %d of %s was trimmed as it was too long\n", i, refFile)
			}
		}
	}
	return []byte(strings.Join(lines, "\n"))

}

func Mkdir(path string) error {
	// run shell `wget URL -O filepath`
	cmd := exec.Command("mkdir", "-p", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func Move(pathSrc string, pathDst string) error {
	cmd := exec.Command("mv", pathSrc, pathDst)
	cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr
	return cmd.Run()
}
func Rm(file string) error {
	// run shell `wget URL -O filepath`
	cmd := exec.Command("rm", file)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func RmForce(folder string) error {
	// run shell `wget URL -O filepath`
	cmd := exec.Command("rm", "-r", folder)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func UnzipFile(path string, dest string) error {
	cmd := exec.Command("unzip", "-PSecret", "-n", path, "-d", dest)
	return cmd.Run()

}
func UntarFile2(path string, dest string) error {
	cmd := exec.Command("tar", "-axf", path, "-C", dest)
	return cmd.Run()

}
func UntarFile(path string, dest string) error {
	cmd := exec.Command("tar", "-xf", path, "-C", dest)
	return cmd.Run()

}

func UnGemFile(path string, dest string) error {
	cmd := exec.Command("gem", "unpack", path, "--target=", dest)
	return cmd.Run()

}

func UnrarFile(path string, dest string) error {
	cmd := exec.Command("unrar", "x", "-y", path, dest)
	return cmd.Run()

}

func UnXZFile(path string, dest string) error {
	cmd := exec.Command("xz", "-d", path, dest)
	return cmd.Run()

}

func GUnzip(path string, dest string) error {
	cmd := exec.Command("gunzip", "-d", path, dest)
	return cmd.Run()

}

/*
{"7z", "7z e -p1 -y"},

		{"gem", "gem unpack"},

		{"rar", "unrar x -y"},

		{"tar", "tar -xf"},

		{"tar.xz","tar -axf"},
		{"tar.gz","tar -axf"},
		{"tgz",   "tar -axf"},
		{"bz2",   "tar -axf"},

		{"war", "unzip -Psecret -n"},
		{"whl", "unzip -Psecret -n"},
		{"zip", "unzip -Psecret -n"},
	  {"nupkg", "unzip -Psecret -n"}
		{"jar", "unzip -Psecret -n"},
		{"aar", "unzip -Psecret -n"},
		{"egg", "unzip -Psecret -n"},


		{"xz", "xz -d"},

		{"gz", "gunzip"},
		;*/

func Clean_dir(path string) error {
	cmd := exec.Command("rm", "-r", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()

}

func (fw *FileWalker) Visit(path string, f os.FileInfo, err error) error {
	if err != nil {
		log.Println(err) // puede ocurrir un error al acceder a un directorio
		return nil
	}
	if f.IsDir() {
		return nil // ignorar directorios
	}
	if f.Mode()&os.ModeSymlink != 0 {
		return nil // ignorar enlaces simbólicos
	}
	fw.Files = append(fw.Files, path)
	return nil
}

func (fw *EncodedFileWalker) Visit(filePath string, f os.FileInfo, err error) error {
	if err != nil {
		log.Println(err) // puede ocurrir un error al acceder a un directorio
		return nil
	}
	if f.IsDir() {
		return nil // ignorar directorios
	}
	if f.Mode()&os.ModeSymlink != 0 {
		return nil // ignorar enlaces simbólicos
	}
	if path.Ext(filePath) == ".enc" {
		fw.Files = append(fw.Files, filePath)
	}
	return nil
}

type FileWalker struct {
	Files []string
}

type EncodedFileWalker struct {
	Files []string
}

type FullFileWalker struct {
	Files []FileDesc
}

func (fw *FullFileWalker) Visit(path string, f os.FileInfo, err error) error {
	if err != nil {
		log.Println(err)
		return nil
	}
	if f.IsDir() {
		return nil // ignore folders
	}
	if f.Mode()&os.ModeSymlink != 0 {
		fw.Files = append(fw.Files, FileDesc{Name: path, IsSymlink: true})
		return nil
	}
	fw.Files = append(fw.Files, FileDesc{Name: path, IsSymlink: false})
	return nil
}

func Get_Files(root string) []string {

	fw := new(FileWalker)
	filepath.Walk(root, fw.Visit)
	return fw.Files
}

func GetAllFiles(root string) []FileDesc {

	fw := new(FullFileWalker)
	filepath.Walk(root, fw.Visit)
	return fw.Files
}

var encFiles []string

func GetEncodedFiles(paths []string) []string {
	totalEncFiles := []string{}

	for _, p := range paths {
		efw := new(EncodedFileWalker)
		filepath.Walk(p, efw.Visit)
		totalEncFiles = append(totalEncFiles, efw.Files...)

	}
	return totalEncFiles

}

func IsRoot(path string) bool {

	fi, err := os.Lstat(path)
	// ..check err...
	if err != nil || fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		return false

	} else {

		fileInfo, err := os.Stat(path)
		if err != nil {
			return false
		}

		if fileInfo != nil {
			if fileInfo.IsDir() {

				files, _ := os.ReadDir(path)
				return len(files) == 2 //1 folder + 1 orginal compressed file

			}

		}
	}
	return false
}

func IsText(src []byte) bool {
	if len(src) > 2 {
		src = src[:len(src)-2]
	}
	for r := range src {
		if src[r] < 9 {
			return false
		}
	}
	return true
}

func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

func isValidISO88591(s string) bool {
	for _, r := range s {
		if r > 0xFF {
			return false
		}
	}

	return true
}

func FileMD5(filePath string) ([]byte, error) {
	// Abre el archivo
	file, err := os.Open(filePath)
	if err != nil {
		return []byte{}, fmt.Errorf("Could not open %s", filePath)
	}
	defer file.Close()

	// Crea un nuevo hash MD5
	hash := md5.New()

	// Copia el contenido del archivo al hash en bloques
	if _, err := io.Copy(hash, file); err != nil {
		return []byte{}, fmt.Errorf("Could not calculate hash for %s", filePath)
	}

	// Obtiene el resultado en bytes
	hashInBytes := hash.Sum(nil)
	return hashInBytes, nil
}

const (
	chunkSize = 8192   // Tamaño del bloque a leer, 8 KB
	threshold = 0.0001 // Umbral de caracteres no imprimibles para considerar el archivo binario
)

func FileIsTextFile(filePath string) (bool, error) {
	// Abre el archivo
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("no se pudo abrir el archivo: %v", err)
	}
	defer file.Close()

	// Usa un bufio.Reader para leer el archivo en bloques
	reader := bufio.NewReader(file)
	buffer := make([]byte, chunkSize)
	totalRead := 0

	for {
		// Lee un bloque del archivo
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("no se pudo leer el archivo: %v", err)
		}
		if err == io.EOF {
			break
		}
		totalRead += n
		if n < chunkSize {
			buffer = buffer[:n-1]
		}
		if !IsText(buffer) {
			return false, nil
		}

	}

	return true, nil
}

func LastCharIsNull(filePath string) (bool, error) {
	// Abre el archivo
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("no se pudo abrir el archivo: %v", err)
	}
	defer file.Close()

	// Mueve el puntero de lectura al penúltimo byte
	_, err = file.Seek(-1, io.SeekEnd)
	if err != nil {
		return false, fmt.Errorf("no se pudo mover el puntero de lectura: %v", err)
	}

	// Lee el último byte
	buffer := make([]byte, 1)
	_, err = file.Read(buffer)
	if err != nil {
		return false, fmt.Errorf("no se pudo leer el último byte: %v", err)
	}

	return buffer[0] == 0, nil
}

func FileCRC64(path string) ([]byte, error) {

	filePath := path

	file, err := os.Open(filePath)
	if err != nil {

		return []byte{}, fmt.Errorf("Could not open file for hashing: %v", err)
	}
	defer file.Close()

	// Crea un nuevo hash CRC64
	hash := crc64.New(m.CRCTable)

	// Copia el contenido del archivo al hash en bloques
	if _, err := io.Copy(hash, file); err != nil {

		return []byte{}, fmt.Errorf("Could not calculate CRC64: %v", err)
	}

	// Obtiene el resultado
	crc64Checksum := hash.Sum64()

	bytes := make([]byte, 8)
	// Convierte el uint64 a bytes en orden BigEndian
	binary.BigEndian.PutUint64(bytes, crc64Checksum)

	return bytes, nil
}
