package docker

import (
	"bytes"
	"database/sql"
	"log"
	"net"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"

	_ "github.com/go-sql-driver/mysql"
)

func Command(args ...string) (string, error) {
	log.Println("$ docker ", strings.Join(args, " "))
	var out bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stdout = &out
	cmd.Stderr = ginkgo.GinkgoWriter

	err := cmd.Run()

	return strings.TrimSpace(out.String()), err
}

func RemoveContainer(name string) error {
	_, err := Command("container", "rm", "--force", "--volumes", name)
	return err
}

func RemoveVolume(name string) error {
	_, err := Command("volume", "rm", "--force", name)
	return err
}

func CreateNetwork(name string) error {
	_, err := Command("network", "create", name)
	return err
}

func RemoveNetwork(name string) error {
	_, err := Command("network", "remove", name)

	return err
}

func PullImage(imageRef string) (string, error) {
	return Command("pull", "--quiet", imageRef)
}

func ContainerPort(containerName, portSpec string) (string, error) {
	hostPort, err := Command("container", "port", containerName, portSpec)
	if err != nil {
		return "", err
	}

	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", err
	}

	return port, nil
}

func MySQLDB(containerName, portSpec string) (*sql.DB, error) {
	mysqlPort, err := ContainerPort(containerName, portSpec)
	if err != nil {
		return nil, err
	}

	dsn := "root@tcp(127.0.0.1:" + mysqlPort + ")/"

	return sql.Open("mysql", dsn)
}
