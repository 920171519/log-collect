package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

const copyBufferSize = 128 * 1024

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// copyDeviceToCloud implements the first transfer stage: device -> cloud. It
// uses the SSH/SFTP transport already secured by the collection workflow, so
// remote hosts do not need sshpass, scp or extra credentials.
func copyDeviceToCloud(ctx context.Context, deviceClient, cloudClient *sftp.Client, devicePath, cloudPath string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := deviceClient.Stat(devicePath)
	if err != nil {
		return 0, fmt.Errorf("读取设备归档信息失败：%w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("设备归档路径是目录：%s", devicePath)
	}
	src, err := deviceClient.Open(devicePath)
	if err != nil {
		return 0, fmt.Errorf("打开设备归档失败：%w", err)
	}
	defer src.Close()

	if err := cloudClient.MkdirAll(path.Dir(cloudPath)); err != nil {
		return 0, fmt.Errorf("创建云主机暂存目录失败：%w", err)
	}
	tmpPath := cloudPath + ".part-" + fmt.Sprintf("%d", time.Now().UnixNano())
	dst, err := cloudClient.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("创建云主机暂存文件失败：%w", err)
	}
	cleanup := true
	defer func() {
		_ = dst.Close()
		if cleanup {
			_ = cloudClient.Remove(tmpPath)
		}
	}()

	buf := make([]byte, copyBufferSize)
	written, err := io.CopyBuffer(dst, contextReader{ctx: ctx, r: src}, buf)
	if err != nil {
		return written, fmt.Errorf("设备到云主机传输失败：%w", err)
	}
	if err := dst.Close(); err != nil {
		return written, fmt.Errorf("关闭云主机文件失败：%w", err)
	}
	if err := ctx.Err(); err != nil {
		return written, err
	}
	cloudInfo, err := cloudClient.Stat(tmpPath)
	if err != nil {
		return written, fmt.Errorf("校验云主机暂存文件失败：%w", err)
	}
	if cloudInfo.Size() != info.Size() || written != info.Size() {
		return written, fmt.Errorf("设备到云主机传输大小不一致（设备 %d 字节，云主机 %d 字节）", info.Size(), cloudInfo.Size())
	}
	// Replace the destination only after a complete copy has arrived.
	_ = cloudClient.Remove(cloudPath)
	if err := cloudClient.Rename(tmpPath, cloudPath); err != nil {
		return written, fmt.Errorf("完成云主机暂存文件重命名失败：%w", err)
	}
	cleanup = false
	return written, nil
}

// copyCloudToLocal downloads the staged archive and returns the final local
// path. Partial files are removed on failure.
func copyCloudToLocal(ctx context.Context, cloudClient *sftp.Client, cloudPath, localDir, fileName string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	info, err := cloudClient.Stat(cloudPath)
	if err != nil {
		return "", 0, fmt.Errorf("读取云主机归档信息失败：%w", err)
	}
	src, err := cloudClient.Open(cloudPath)
	if err != nil {
		return "", 0, fmt.Errorf("打开云主机归档失败：%w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(localDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("创建本地目录失败：%w", err)
	}
	finalPath, err := uniqueLocalPath(localDir, fileName)
	if err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(localDir, ".logcollector-*.part")
	if err != nil {
		return "", 0, fmt.Errorf("创建本地临时文件失败：%w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	buf := make([]byte, copyBufferSize)
	written, err := io.CopyBuffer(tmp, contextReader{ctx: ctx, r: src}, buf)
	if err != nil {
		return "", written, fmt.Errorf("云主机到本地传输失败：%w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return "", written, fmt.Errorf("设置本地文件权限失败：%w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", written, fmt.Errorf("写入本地文件失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", written, fmt.Errorf("关闭本地文件失败：%w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", written, err
	}
	if written != info.Size() {
		return "", written, fmt.Errorf("云主机到本地传输大小不一致（云主机 %d 字节，本地 %d 字节）", info.Size(), written)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", written, fmt.Errorf("保存本地归档失败：%w", err)
	}
	cleanup = false
	return finalPath, written, nil
}

func uniqueLocalPath(dir, fileName string) (string, error) {
	fileName = filepath.Base(fileName)
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	if base == "" {
		base = "logs"
	}
	for i := 0; i < 1000; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%d", i+1)
		}
		candidate := filepath.Join(dir, base+suffix+ext)
		_, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("检查本地文件失败：%w", err)
		}
	}
	return "", fmt.Errorf("本地目录中存在过多同名归档：%s", fileName)
}

func removeRemoteFile(client *sftp.Client, remotePath string) error {
	if client == nil || strings.TrimSpace(remotePath) == "" {
		return nil
	}
	if err := client.Remove(remotePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
