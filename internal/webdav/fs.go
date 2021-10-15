package webdav

import (
	"container/list"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/jakeslee/aliyundrive"
	"github.com/jakeslee/aliyundrive/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/webdav"
	"hash"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	CtxSizeValue = "Size"
)

var RapidCache = sync.Map{}
var RapidCacheFolder = sync.Map{}

func NewAliDriveFS(drive *aliyundrive.AliyunDrive, credential *aliyundrive.Credential, rapid bool) webdav.FileSystem {
	logrus.Infof("rapid upload mode: %v", rapid)
	return &aliDriveFS{
		driver:      drive,
		credential:  credential,
		rapidUpload: rapid,
	}
}

type aliDriveFS struct {
	mu          sync.Mutex
	driver      *aliyundrive.AliyunDrive
	credential  *aliyundrive.Credential
	rapidUpload bool
}

func (a *aliDriveFS) mkdir(credential *aliyundrive.Credential, fileId, name string) (string, error) {
	dir, err := a.driver.CreateDirectory(credential, fileId, name)
	if err != nil {
		return "", err
	}

	return dir.FileId, nil
}

func (a *aliDriveFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	dir := aliyundrive.PrefixSlash(filepath.Clean(name))

	fileId, foundPath, err := a.driver.ResolvePathToFileId(a.credential, dir)

	if err != nil && foundPath != "" {
		left := aliyundrive.RemovePrefixSlash(dir[len(foundPath):])

		splits := strings.Split(left, "/")

		for _, folder := range splits {
			id, err := a.mkdir(a.credential, fileId, folder)

			if err != nil {
				return err
			}

			fileId = id
		}
	}

	return nil
}

func (a *aliDriveFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.rapidUpload && flag&os.O_CREATE == 0 {
		if cacheFile, ok := RapidCache.Load(name); ok {
			logrus.Infof("reqeusted file %s hit local cached file", name)

			file, err := os.OpenFile(cacheFile.(string), flag, perm)
			if err != nil {
				return nil, err
			}
			return file, nil
		}
	}

	size := ctx.Value(CtxSizeValue).(int64)

	fileId, path, err := a.driver.ResolvePathToFileId(a.credential, name)

	// 找不到任何前缀文件，错误
	if err != nil && path == "" {
		return nil, err
	}

	exist := false

	if err == nil {
		exist = true
	}

	file, err := a.driver.GetFile(a.credential, fileId)
	if err != nil {
		return nil, err
	}

	if flag&os.O_CREATE != 0 {
		// 创建时，如果文件已经存在，大小相同不再上传
		//if exist && size == file.Size {
		//	return nil, os.ErrExist
		//}

		// 否则删除文件，重新上传
		if exist {
			_, err := a.driver.RemoveFile(a.credential, fileId)
			if err != nil {
				return nil, err
			}
			fileId = file.ParentFileId
		}

		fileName := filepath.Base(name)

		if fileName == ".DS_Store" {
			return nil, os.ErrInvalid
		}

		_file := &aliFile{
			n: &aliFileInfo{
				size:         size,
				name:         fileName,
				mode:         perm,
				modTime:      time.Now(),
				parentFileId: fileId,
			},
			driver:      a.driver,
			credential:  a.credential,
			enableRapid: a.rapidUpload,
			fullPath:    name,
		}

		if a.rapidUpload {
			tempFile, err := ioutil.TempFile("", "*."+fileName)
			if err != nil {
				return nil, err
			}

			_file.rapid.hash = sha1.New()
			_file.rapid.file = tempFile
			_file.rapid.writer = io.MultiWriter(_file.rapid.hash, tempFile)

			return _file, nil
		}

		if size <= 0 {
			return nil, os.ErrInvalid
		}

		reader, writer := io.Pipe()

		_file.create.reader = reader
		_file.create.writer = writer

		go func() {
			_, err := a.driver.UploadFile(a.credential, &aliyundrive.UploadFileOptions{
				Name:         fileName,
				Size:         size,
				ParentFileId: fileId,
				Reader:       reader,
				ProgressStart: func(info *aliyundrive.ProgressInfo) {
					_file.n.fileId = info.FileId
				},
			})
			if err != nil {
				logrus.Errorf("upload file error %s", err)
				ctx.Done()
			}

			a.mu.Lock()
			defer a.mu.Unlock()
			_file.create.finished = true
		}()
		return _file, nil
	}

	// 除非是创建，否则不能打开不存在的（此时部分匹配路径）
	if !exist {
		return nil, os.ErrNotExist
	}

	fileInfo := NewAliFileInfo(file.File)
	fileRes := &aliFile{
		n:           fileInfo.(*aliFileInfo),
		driver:      a.driver,
		credential:  a.credential,
		fullPath:    name,
		enableRapid: a.rapidUpload,
	}

	return fileRes, nil
}

type aliFile struct {
	n              *aliFileInfo
	fullPath       string
	mu             sync.Mutex
	driver         *aliyundrive.AliyunDrive
	credential     *aliyundrive.Credential
	nextMarker     string
	lastFetchItems []*models.File
	pos            int64
	reader         io.ReadCloser
	readerClosed   bool
	create         struct {
		writePos int64
		reader   io.Reader
		writer   io.Writer
		finished bool
	}
	// 用于秒传
	enableRapid bool
	rapid       struct {
		hash     hash.Hash
		file     *os.File
		writer   io.Writer
		finished bool
	}
}

func (a *aliFile) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 常规上传
	if !a.create.finished && a.create.writer != nil {
		err := a.create.writer.(*io.PipeWriter).Close()
		if err != nil {
			return err
		}
	}

	// 秒传
	if !a.rapid.finished && a.rapid.file != nil {
		a.uploadFinished()
	}

	a.closeReader()

	return nil
}

func (a *aliFile) Read(p []byte) (n int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.n.mode.IsDir() {
		return 0, os.ErrInvalid
	}

	if a.reader == nil {
		bytesRange := fmt.Sprintf("bytes=%d-", a.pos)

		response, err := a.driver.Download(a.credential, a.n.file.FileId, bytesRange)
		if err != nil {
			return 0, err
		}

		a.reader = response.Body
		a.readerClosed = false
	}

	n, err = a.reader.Read(p)

	a.pos += int64(n)

	if a.pos >= a.n.size {
		a.closeReader()
		return n, io.EOF
	}

	return
}

func (a *aliFile) Seek(offset int64, whence int) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	npos := a.pos

	switch whence {
	case io.SeekStart:
		if offset != npos {
			a.closeReader()
		}

		npos = offset

		logrus.Debugf("file: %s seek %d", a.n.name, npos)
	case io.SeekCurrent:
		npos += offset
	case io.SeekEnd:
		npos = a.n.size - offset
	default:
		npos = -1
	}
	if npos < 0 {
		return 0, os.ErrInvalid
	}

	if a.pos == npos {
		return a.pos, nil
	}

	a.pos = npos

	return a.pos, nil
}

func (a *aliFile) closeReader() {
	if a.reader != nil && !a.readerClosed {
		_ = a.reader.Close()
		a.reader = nil
		a.readerClosed = true
	}
}

func (a *aliFile) Readdir(count int) ([]fs.FileInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.n.Mode().IsDir() {
		return nil, os.ErrInvalid
	}

	result := make([]fs.FileInfo, 0, 10)
	resultMap := make(map[string]fs.FileInfo)

	if a.enableRapid {
		if filesInterface, ok := RapidCacheFolder.Load(a.n.fileId); ok {
			if files, ok := filesInterface.(*list.List); ok {
				for i := files.Front(); i != nil; i = i.Next() {
					info := i.Value.(*aliFileInfo)

					result = append(result, info)
					resultMap[info.name] = info
				}
			}
		}
	}

	// 取目录全部列表
	if count <= 0 {
		marker := ""

		for {
			files, err := a.driver.GetFolderFiles(a.credential, &aliyundrive.FolderFilesOptions{
				OrderBy:        "updated_at",
				OrderDirection: models.OrderDirectionTypeDescend,
				FolderFileId:   a.n.file.FileId,
				Marker:         marker,
			})

			if err != nil {
				return nil, err
			}

			for _, item := range files.Items {
				if _, ok := resultMap[item.Name]; !ok {
					result = append(result, NewAliFileInfo(item))
				}
			}

			if files.NextMarker == "" {
				break
			}

			marker = files.NextMarker
		}

		return result, nil
	}

	for {
		for ; a.pos < int64(len(a.lastFetchItems)) && count > 0; a.pos++ {
			count--

			item := a.lastFetchItems[a.pos]
			if _, ok := resultMap[item.Name]; !ok {
				result = append(result, NewAliFileInfo(item))
			}
		}

		if count <= 0 {
			break
		}

		files, err := a.driver.GetFolderFiles(a.credential, &aliyundrive.FolderFilesOptions{
			OrderBy:        "updated_at",
			OrderDirection: models.OrderDirectionTypeDescend,
			FolderFileId:   a.n.file.FileId,
			Marker:         a.nextMarker,
		})

		if err != nil {
			return nil, err
		}

		a.pos = 0
		a.lastFetchItems = files.Items
		a.nextMarker = files.NextMarker
	}

	return result, nil
}

func (a *aliFile) Stat() (fs.FileInfo, error) {
	return a.n, nil
}

func (a *aliFile) rapidWrite(p []byte) (n int, err error) {
	n, err = a.rapid.writer.Write(p)
	if err != nil {
		logrus.Errorf("upload %s error %s", a.n.name, err)
		return n, err
	}

	a.pos += int64(n)

	return n, nil
}

func (a *aliFile) uploadFinished() {
	logrus.Infof("upload %s finished. size: %d/%d, start rapid process...", a.n.name, a.pos, a.n.size)
	logrus.Infof("%s temporary stores in %s", a.n.name, a.rapid.file.Name())

	RapidCache.Store(a.fullPath, a.rapid.file.Name())
	files, _ := RapidCacheFolder.LoadOrStore(a.n.parentFileId, list.New())
	l := files.(*list.List)
	l.PushBack(a.n)

	if a.n.size <= 0 {
		a.n.size = a.pos
	}

	go func() {
		_hash := fmt.Sprintf("%x", a.rapid.hash.Sum(nil))
		defer func(file *os.File) {
			_ = file.Close()
			time.AfterFunc(1 * time.Minute, func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				logrus.Debugf("remove file's local cache: %s", a.fullPath)

				for i := l.Front(); i != nil; i = i.Next() {
					value := i.Value.(fs.FileInfo)
					if value.Name() == a.n.name {
						l.Remove(i)
					}
				}

				RapidCache.Delete(a.fullPath)

				err := os.Remove(file.Name())
				if err != nil {
					logrus.Warnf("remove temp file error %s", err)
				}
			})
		}(a.rapid.file)

		fileRapid, rapid, err := a.driver.UploadFileRapid(a.credential, &aliyundrive.UploadFileRapidOptions{
			UploadFileOptions: aliyundrive.UploadFileOptions{
				Name:         a.n.name,
				Size:         a.n.size,
				ParentFileId: a.n.parentFileId,
			},
			File:        a.rapid.file,
			ContentHash: _hash,
		})

		if err != nil {
			logrus.Errorf("rapid upload fail, error %s", err)
			return
		}

		logrus.Infof("upload %s finished, rapid mode: %v, fileId %s", a.n.name, rapid, fileRapid.FileId)
		a.n.file = fileRapid
		a.n.fileId = fileRapid.FileId
		a.rapid.finished = true
	}()
}

func (a *aliFile) Write(p []byte) (n int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.n.IsDir() {
		return 0, os.ErrInvalid
	}

	if a.enableRapid {
		return a.rapidWrite(p)
	}

	if a.create.writer != nil {
		n, err = a.create.writer.Write(p)
		if err != nil {
			logrus.Errorf("upload %s error %s", a.n.name, err)
			return n, err
		}

		a.create.writePos += int64(n)

		logrus.Debugf("uploaded %d, writepos: %d", n, a.create.writePos)
		return n, nil
	}

	return 0, errors.New("cannot write, writer is nil")
}

func (a *aliDriveFS) RemoveAll(ctx context.Context, name string) error {
	a.mu.Lock()
	a.mu.Unlock()

	fileId, _, err := a.driver.ResolvePathToFileId(a.credential, name)
	if err != nil {
		return err
	}

	logrus.Warnf("removing %s: %s", fileId, name)

	_, err = a.driver.RemoveFile(a.credential, fileId)
	if err != nil {
		return err
	}

	return nil
}

func (a *aliDriveFS) Rename(ctx context.Context, oldName, newName string) error {
	logrus.Infof("rename file %s to %s", oldName, newName)

	fileId, _, err := a.driver.ResolvePathToFileId(a.credential, oldName)
	if err != nil {
		logrus.Errorf("resolve file %s, err: %s", oldName, err)
		return os.ErrNotExist
	}

	oldDir, oldFileName := filepath.Split(filepath.Clean(oldName))
	toDir, name := filepath.Split(filepath.Clean(newName))

	toFileId, found, err := a.driver.ResolvePathToFileId(a.credential, newName)

	// 目标已存在，取消
	if err == nil {
		file, err := a.driver.GetFile(a.credential, toFileId)
		if err != nil {
			return err
		}

		// 如果目标是目录，则是移动逻辑
		if file.Type != models.FileTypeFolder {
			return os.ErrExist
		}

		toDir = newName
		name = oldFileName
	}

	// 目标路径不存在，创建路径
	if found != toDir {
		logrus.Debugf("dest path not exist, found %s", found)
		err := a.Mkdir(ctx, toDir, os.ModeDir)
		if err != nil {
			logrus.Errorf("mkdir %s, err %s", toDir, err)
			return err
		}

		toFileId, _, err = a.driver.ResolvePathToFileId(a.credential, toDir)
		if err != nil {
			logrus.Errorf("resolve file %s, err: %s", toDir, err)
			return err
		}
	}

	// 目标路径和当前路径不同，先移动过去
	if oldDir != toDir {
		logrus.Infof("dest not in current dir, moving %s to %s", oldName, toDir)
		_, err := a.driver.MoveFile(a.credential, fileId, toFileId)
		if err != nil {
			logrus.Errorf("moving file %s to %s, err: %s", oldName, toFileId, err)
			return err
		}
	}

	// 如果文件名不同，重命名
	if oldFileName != name {
		_, err := a.driver.RenameFile(a.credential, fileId, name)
		if err != nil {
			logrus.Errorf("renaming file %s to %s, err: %s", fileId, name, err)
			return err
		}
	}

	return nil
}

func (a *aliDriveFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if a.rapidUpload {
		if _fileName, ok := RapidCache.Load(name); ok {
			stat, err := os.Stat(_fileName.(string))
			if err != nil {
				logrus.Warnf("read cached file error %s", err)
				RapidCache.Delete(name)
			} else {
				return stat, nil
			}
		}
	}

	fileId, _, err := a.driver.ResolvePathToFileId(a.credential, name)
	if err != nil {
		if err == aliyundrive.ErrPartialFoundPath {
			return nil, os.ErrNotExist
		}

		return nil, err
	}

	file, err := a.driver.GetFile(a.credential, fileId)
	if err != nil {
		return nil, err
	}

	return NewAliFileInfo(file.File), nil
}

func NewAliFileInfo(file *models.File) os.FileInfo {
	mode := os.ModePerm

	if file.Type == models.FileTypeFolder {
		mode |= os.ModeDir
	}

	return &aliFileInfo{
		parentFileId: file.ParentFileId,
		fileId:       file.FileId,
		file:         file,
		name:         file.Name,
		size:         file.Size,
		mode:         mode,
		modTime:      file.UpdatedAt,
	}
}

type aliFileInfo struct {
	fileId       string
	parentFileId string
	file         *models.File
	name         string
	size         int64
	mode         os.FileMode
	modTime      time.Time
}

func (f *aliFileInfo) Name() string       { return f.name }
func (f *aliFileInfo) Size() int64        { return f.size }
func (f *aliFileInfo) Mode() os.FileMode  { return f.mode }
func (f *aliFileInfo) ModTime() time.Time { return f.modTime }
func (f *aliFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f *aliFileInfo) Sys() interface{}   { return nil }
