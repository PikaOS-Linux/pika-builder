package activities

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pkbldr/packages"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func UpdateDockerContainer(ctx context.Context) error {
	start := time.Now()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	// Specify docker image and container name
	imageName := "ghcr.io/pikaos-linux/pika-base-debian-container:latest"
	containerName := "pikaos-bldr-container"

	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	hostDir := filepath.Join(workingDir, "temppackagesdir")
	containerDir := "/data" // Mount location inside the container

	// Create local directory if it doesn't exist
	if _, err := os.Stat(hostDir); os.IsNotExist(err) {
		err = os.MkdirAll(hostDir, 0777) // Change permissions if needed
		if err != nil {
			return err
		}
	}

	fmt.Println("Pulling image...")
	out, err := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer out.Close()
	io.Copy(os.Stdout, out)

	// Create the container
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      imageName,
		WorkingDir: containerDir,
		Cmd:        []string{"tail", "-f", "/dev/null"}, // Keep container running
		Tty:        true,                                // Allocate a pseudo-TTY
	}, &container.HostConfig{
		Privileged: true,
		Binds:      []string{fmt.Sprintf("%s:%s", hostDir, containerDir)},
	}, nil, nil, containerName)
	if err != nil {
		return err
	}

	// Start the container
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	command := "apt-get update -y && apt-get upgrade -y && apt-get autoremove -y && pika-pbuilder-amd64-init"

	// Execute the command
	execResp, err := cli.ContainerExecCreate(ctx, resp.ID, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-c", command},
		Tty:          true,
		Privileged:   true,
	})
	if err != nil {
		return err
	}

	// Attach to the command's output
	output, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true})
	if err != nil {
		return err
	}
	defer output.Close()

	// Stream the command's output to your console
	io.Copy(os.Stdout, output.Reader)

	cli.ContainerStop(ctx, resp.ID, container.StopOptions{})
	_, err = cli.ContainerCommit(ctx, resp.ID, types.ContainerCommitOptions{Reference: "pikaos-bldr-container:latest"})
	if err != nil {
		return err
	}

	// Clean up (optional - you might want to keep the container)
	fmt.Println("Stopping and removing container...")
	cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{})
	fmt.Printf("Update loop took %s\n", time.Since(start))
	return nil

}

func StartBuildLoop(ctx context.Context, pkgsToBuild []packages.PackageInfo) error {

	start := time.Now()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	// Specify docker image and container name
	imageName := "pikaos-bldr-container:latest"
	containerName := "pikaos-bldr-container"

	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	hostDir := filepath.Join(workingDir, "temppackagesdir")
	containerDir := "/data" // Mount location inside the container

	// Create local directory if it doesn't exist
	if _, err := os.Stat(hostDir); os.IsNotExist(err) {
		err = os.MkdirAll(hostDir, 0755) // Change permissions if needed
		if err != nil {
			return err
		}
	}

	// Create the container
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      imageName,
		WorkingDir: containerDir,
		Cmd:        []string{"tail", "-f", "/dev/null"}, // Keep container running
		Tty:        true,                                // Allocate a pseudo-TTY
	}, &container.HostConfig{
		Privileged: true,
		Binds:      []string{fmt.Sprintf("%s:%s", hostDir, containerDir)},
	}, nil, nil, containerName)
	if err != nil {
		return err
	}

	// Start the container
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	// Loop through the packages and build them
	for _, pkg := range pkgsToBuild {
		dir, err := os.MkdirTemp(hostDir, pkg.Name)
		if err != nil {
			return err
		}

		pkgdirs := strings.Split(dir, "/")
		pkgdir := pkgdirs[len(pkgdirs)-1]

		command := "cd " + pkgdir + " && apt-get source " + pkg.Name + " -y"
		// Execute the command
		execResp, err := cli.ContainerExecCreate(ctx, resp.ID, types.ExecConfig{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          []string{"sh", "-c", command},
			Tty:          true,
			Privileged:   true,
		})
		if err != nil {
			return err
		}

		// Attach to the command's output
		output, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true})
		if err != nil {
			return err
		}
		defer output.Close()

		// Stream the command's output to your console
		io.Copy(os.Stdout, output.Reader)

		// Command to execute inside the container
		command = "cd " + pkgdir + " && pika-pbuilder-amd64-v3-lto-build *.dsc"

		// Execute the command
		execResp, err = cli.ContainerExecCreate(ctx, resp.ID, types.ExecConfig{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          []string{"sh", "-c", command},
			Tty:          true,
			Privileged:   true,
		})
		if err != nil {
			return err
		}

		// Attach to the command's output
		output, err = cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true})
		if err != nil {
			return err
		}
		defer output.Close()

		// Stream the command's output to your console
		io.Copy(os.Stdout, output.Reader)

		buildErr := true
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".deb" {
				buildErr = false
			}
		}
		if buildErr {
			fmt.Println("Build failed for " + pkg.Name)
		} else {
			fmt.Println("Build succeeded for " + pkg.Name)
		}

		os.RemoveAll(dir)
	}

	// Clean up (optional - you might want to keep the container)
	fmt.Println("Stopping and removing container...")
	cli.ContainerStop(ctx, resp.ID, container.StopOptions{})
	cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{})
	fmt.Printf("Build loop took %s\n", time.Since(start))
	return nil
}
