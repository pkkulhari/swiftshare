package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mappu/miqt/qt"
)

const (
	BUFFER_SIZE = 1024 * 1024 * 32 // 32 MB buffer
	PORT        = ":8010"
)

type Swiftshare struct {
	app         *qt.QApplication
	window      *qt.QMainWindow
	sendButton  *qt.QPushButton
	recvButton  *qt.QPushButton
	statusLabel *qt.QLabel
	discovery   *Discovery
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewSwiftshare() *Swiftshare {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Swiftshare{
		app:       qt.NewQApplication(os.Args),
		discovery: NewDiscovery(),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start service discovery
	hostname, _ := os.Hostname()
	err := s.discovery.RegisterService(hostname)
	if err != nil {
		fmt.Printf("Failed to register zeroconf service: %v\n", err)
	}

	err = s.discovery.StartDiscovery(ctx)
	if err != nil {
		fmt.Printf("Failed to start service discovery: %v\n", err)
	}

	return s
}

func (s *Swiftshare) setupUI() {
	// Create main window
	s.window = qt.NewQMainWindow(nil)
	s.window.SetWindowTitle("Swift Share")
	s.window.SetMinimumSize2(400, 300)

	// Create central widget and layout
	centralWidget := qt.NewQWidget(nil)
	layout := qt.NewQVBoxLayout(centralWidget)
	s.window.SetCentralWidget(centralWidget)

	// Create buttons and status label
	s.sendButton = qt.NewQPushButton2()
	s.sendButton.SetText("Send File")
	s.recvButton = qt.NewQPushButton2()
	s.recvButton.SetText("Receive File")
	s.statusLabel = qt.NewQLabel2()
	s.statusLabel.SetText("Ready to transfer files")

	// Add qt to layout
	layout.AddWidget(s.sendButton.QWidget)
	layout.AddWidget(s.recvButton.QWidget)
	layout.AddWidget(s.statusLabel.QWidget)
	layout.AddStretch()

	// Connect signals
	s.sendButton.OnPressed(func() { s.sendFileHandler() })
	s.recvButton.OnPressed(func() { s.receiveFileHandler() })
}

func (s *Swiftshare) sendFileHandler() {
	fd := qt.NewQFileDialog(nil)
	fd.SetWindowTitle("Select File")
	fd.SetFileMode(qt.QFileDialog__ExistingFile)
	fd.SetAcceptMode(qt.QFileDialog__AcceptOpen)
	fd.Exec()

	if fd.Result() != int(qt.QDialog__Accepted) {
		return
	}

	filePaths := fd.SelectedFiles()
	if len(filePaths) == 0 {
		return
	}
	filePath := filePaths[0]

	go s.sendFile(filePath)
}

func (a *Swiftshare) sendFile(filePath string) {
	// Update UI
	a.updateStatus(fmt.Sprintf("Preparing to send file: %s", filepath.Base(filePath)))

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error opening file: %v", err))
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error getting file info: %v", err))
		return
	}

	// Listen for receiver with TCP optimizations
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				setSocketOpts(fd)
			})
		},
	}

	listener, err := lc.Listen(context.Background(), "tcp", PORT)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error creating listener: %v", err))
		return
	}
	defer listener.Close()

	a.updateStatus(fmt.Sprintf("Waiting for receiver to connect at %s", listener.Addr().String()))

	// Accept connection
	conn, err := listener.Accept()
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error accepting connection: %v", err))
		return
	}
	defer conn.Close()

	// Set TCP keep-alive
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(30 * time.Second)

	// Send file metadata
	fileName := filepath.Base(filePath)
	fileSize := fileInfo.Size()
	metaData := fmt.Sprintf("%s|%d", fileName, fileSize)
	_, err = conn.Write([]byte(metaData))
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error sending file metadata: %v", err))
		return
	}

	// Create a custom reader to track progress
	startTime := time.Now()
	progressReader := &ProgressReader{
		Reader: file,
		Total:  fileSize,
		OnProgress: func(bytesRead int64) {
			progress := float64(bytesRead) / float64(fileSize) * 100
			elapsedTime := time.Since(startTime)
			speed := float64(bytesRead) / (1024 * 1024) / elapsedTime.Seconds()
			bytesTransferred := float64(bytesRead) / (1024 * 1024) // Convert to MB

			a.updateStatus(fmt.Sprintf(
				"Sending: %.2f%% (%.2f MB / %.2f MB) | Speed: %.2f MB/s",
				progress, bytesTransferred, float64(fileSize)/(1024*1024), speed,
			))
		},
	}

	// Use io.Copy for efficient transfer
	_, err = io.Copy(conn, progressReader)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error during file transfer: %v", err))
		return
	}

	a.updateStatus(fmt.Sprintf("File sent successfully: %s", fileName))
}

func (s *Swiftshare) receiveFileHandler() {
	// Create dialog for service selection
	dialog := qt.NewQDialog(nil)
	dialog.SetWindowTitle("Select Device")
	dialog.SetMinimumWidth(300)

	layout := qt.NewQVBoxLayout(nil)
	deviceList := qt.NewQListWidget(nil)
	layout.AddWidget(deviceList.QWidget)
	dialog.SetLayout(layout.QLayout)

	// Add buttons
	buttonBox := qt.NewQDialogButtonBox(nil)
	buttonBox.SetStandardButtons(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel)
	layout.AddWidget(buttonBox.QWidget)

	deviceMap := make(map[string]string) // Map display name to IP address

	// Handle discovered services
	go func() {
		for entry := range s.discovery.GetEntries() {
			displayName := fmt.Sprintf("%s (%s)", entry.Instance, entry.AddrIPv4[0].String())
			deviceMap[displayName] = s.discovery.GetIPAddress(entry)
			deviceList.AddItem(displayName)
		}
	}()

	buttonBox.OnAccepted(func() {
		currentItem := deviceList.CurrentItem()
		if currentItem != nil {
			displayName := currentItem.Text()
			if ipAddress, ok := deviceMap[displayName]; ok {
				dialog.Accept()
				go s.receiveFile(ipAddress)
			}
		}
	})

	dialog.Exec()
}

func (a *Swiftshare) receiveFile(ipAddress string) {
	a.updateStatus(fmt.Sprintf("Connecting to sender %s...", ipAddress))

	// Connect to sender
	conn, err := net.Dial("tcp", ipAddress+PORT)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error connecting: %v", err))
		return
	}
	defer conn.Close()

	// Read file metadata
	metaBuffer := make([]byte, 1024)
	n, err := conn.Read(metaBuffer)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error reading metadata: %v", err))
		return
	}

	// Parse metadata
	metadata := string(metaBuffer[:n])
	parts := strings.Split(metadata, "|")
	if len(parts) != 2 {
		a.updateStatus("Invalid metadata received")
		return
	}

	fileName := parts[0]
	fileSize, _ := strconv.ParseInt(parts[1], 10, 64)

	// Create file for writing
	outputPath := filepath.Join(getDownloadsDir(), fileName)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error creating file: %v", err))
		return
	}
	defer outputFile.Close()

	// Create a ProgressWriter to track progress
	startTime := time.Now()
	progressWriter := &ProgressWriter{
		Writer: outputFile,
		Total:  fileSize,
		OnProgress: func(bytesWritten int64) {
			progress := float64(bytesWritten) / float64(fileSize) * 100
			elapsedTime := time.Since(startTime)
			speed := float64(bytesWritten) / (1024 * 1024) / elapsedTime.Seconds()
			bytesTransferred := float64(bytesWritten) / (1024 * 1024) // Convert to MB

			a.updateStatus(fmt.Sprintf(
				"Receiving: %.2f%% (%.2f MB / %.2f MB) | Speed: %.2f MB/s",
				progress, bytesTransferred, float64(fileSize)/(1024*1024), speed,
			))
		},
	}

	// Use io.Copy for efficient transfer
	_, err = io.Copy(progressWriter, conn)
	if err != nil {
		a.updateStatus(fmt.Sprintf("Error during file transfer: %v", err))
		return
	}

	a.updateStatus(fmt.Sprintf("File received: %s", outputPath))
}

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	BytesRead  int64
	OnProgress func(int64)
}

// Read implements the io.Reader interface
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.BytesRead += int64(n)
	if pr.OnProgress != nil {
		pr.OnProgress(pr.BytesRead)
	}
	return n, err
}

// ProgressWriter wraps an io.Writer to track progress
type ProgressWriter struct {
	Writer       io.Writer
	Total        int64
	BytesWritten int64
	OnProgress   func(int64)
}

// Write implements the io.Writer interface
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.BytesWritten += int64(n)
	if pw.OnProgress != nil {
		pw.OnProgress(pw.BytesWritten)
	}
	return n, err
}

func (s *Swiftshare) updateStatus(message string) {
	s.statusLabel.SetText(message)
}

func (s *Swiftshare) Run() {
	s.setupUI()
	s.window.Show()
	qt.QApplication_Exec()
	s.discovery.Shutdown() // Cleanup zeroconf when app closes
	s.cancel()             // Cancel discovery context
}

func main() {
	app := NewSwiftshare()
	app.Run()
}

// Utils
func getDownloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "downloads"
	}
	return filepath.Join(home, "Downloads")
}
