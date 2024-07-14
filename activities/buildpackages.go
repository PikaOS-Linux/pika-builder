package activities

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"pkbldr/config"
	"pkbldr/packages"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"golang.org/x/exp/slog"
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

	cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})
	forceKillContainers(ctx, cli, containerName)
	cli.ImageRemove(ctx, imageName, types.ImageRemoveOptions{Force: true, PruneChildren: true})

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

func StartBuildLoop(ctx context.Context) error {

	pkgsToBuild := packages.GetBuildQueue()
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

	forceKillContainers(ctx, cli, containerName)
	containers, err := createContainers(ctx, cli, containerName, hostDir, containerDir, imageName)
	if err != nil {
		return err
	}

	for _, pkg := range pkgsToBuild {
		for _, pkg2 := range pkg {
			pkg2.Status = packages.Queued
			packages.UpdatePackage(pkg2, false)
		}
	}

	fmt.Println("Build loop started")
	// Loop through the packages and build them
	err = buildBatch(pkgsToBuild, cli, containers, hostDir)
	if err != nil {
		return err
	}

	// Clean up (optional - you might want to keep the container)
	fmt.Println("Stopping and removing container...")
	for _, containerID := range containers {
		cli.ContainerStop(ctx, containerID, container.StopOptions{})
		cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{})
	}
	fmt.Printf("Build loop took %s\n", time.Since(start))
	return nil
}

func createContainers(ctx context.Context, cli *client.Client, containerName string, hostDir string, containerDir string, imageName string) ([]string, error) {
	containers := make([]string, 0)
	for i := 0; i < 3; i++ {
		resp, err := cli.ContainerCreate(ctx, &container.Config{
			Image:      imageName,
			WorkingDir: containerDir,
			Cmd:        []string{"tail", "-f", "/dev/null"}, // Keep container running
			Tty:        true,                                // Allocate a pseudo-TTY
		}, &container.HostConfig{
			Privileged: true,
			Binds:      []string{fmt.Sprintf("%s:%s", hostDir, containerDir)},
		}, nil, nil, containerName+"-"+strconv.Itoa(i))
		if err != nil {
			return nil, err
		}

		// Start the container
		if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
			return nil, err
		}
		containers = append(containers, resp.ID)
	}

	return containers, nil
}

func forceKillContainers(ctx context.Context, cli *client.Client, containerName string) {
	for i := 0; i < 3; i++ {
		cli.ContainerRemove(ctx, containerName+"-"+strconv.Itoa(i), types.ContainerRemoveOptions{Force: true})
	}
}

func buildBatch(packs packages.PackageBuildQueue, cli *client.Client, containers []string, hostDir string) error {
	packageQueue := make(chan []packages.PackageInfo, 3)
	// Create a worker pool with 3
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		cont := containers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case pack, ok := <-packageQueue:
					if !ok {
						return
					}
					err := buildPackage(context.Background(), pack, cli, cont, hostDir)
					if err != nil {
						slog.Error(err.Error())
					}
				default:
					// No more packages to process, exit the goroutine
					return
				}
			}
		}()
	}

	// Add the packages to the queue
	for _, v := range packs {
		packageQueue <- v
	}

	// Close the queue to signal the workers to stop
	close(packageQueue)

	// Wait for all the workers to finish
	wg.Wait()
	return nil
}

func buildPackage(ctx context.Context, pkgs []packages.PackageInfo, cli *client.Client, respid string, hostDir string) error {
	pkg := pkgs[0]
	buildVersion := pkg.PendingVersion
	if buildVersion == "" {
		buildVersion = pkg.Version
	}
	for _, pkg2 := range pkgs {
		pkg2.Status = packages.Building
		pkg2.LastBuildVersion = buildVersion
		packages.UpdatePackage(pkg2, false)
	}

	// Create a temporary directory for the package
	dir, err := os.MkdirTemp(hostDir, pkg.Name)
	if err != nil {
		return err
	}

	pkgdirs := strings.Split(dir, "/")
	pkgdir := pkgdirs[len(pkgdirs)-1]

	command := "cd " + pkgdir + " && eatmydata apt-get source " + pkg.Name + "=" + buildVersion + " -y"
	// Execute the command
	execResp, err := cli.ContainerExecCreate(ctx, respid, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-c", command},
		Tty:          true,
		Privileged:   true,
	})
	if err != nil {
		buildError(pkgs, err, dir)
		return nil
	}

	// Attach to the command's output
	output, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true})
	if err != nil {
		buildError(pkgs, err, dir)
		return nil
	}

	io.Copy(io.Discard, output.Reader)
	output.Close()

	buildcmd := "pika-pbuilder-amd64-v3-lto-build"
	if config.Configs.LTOBlocklist != nil && slices.Contains(config.Configs.LTOBlocklist, pkg.Name) {
		buildcmd = "pika-pbuilder-amd64-v3-build"
	} else if pkg.BuildAttempts > 2 && pkg.LastBuildStatus == packages.Built {
		buildcmd = "find ./ -type f -name '*.dsc' bash -c 'dget -all ' {} \\;"
	} else if pkg.BuildAttempts > 0 && pkg.LastBuildStatus == packages.Error {
		buildcmd = "pika-pbuilder-amd64-v3-build"
	}

	// Command to execute inside the container
	command = "cd " + pkgdir + " && " + buildcmd + " *.dsc"

	// Execute the command
	execResp, err = cli.ContainerExecCreate(ctx, respid, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-c", command},
		Tty:          true,
		Privileged:   true,
	})
	if err != nil {
		buildError(pkgs, err, dir)
		return nil
	}

	timer := time.NewTicker(time.Minute * 10)
	numChecks := 0

	// start the command
	output, err = cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true})
	if err != nil {
		buildError(pkgs, err, dir)
		return nil
	}
	timeoutHit := false

	go func() {
		for {
			select {
			case <-timer.C:
				success := checkBuild(pkgs, pkg, dir)
				if !success {
					numChecks++
					if numChecks > 6 {
						fmt.Println("Timeout reached for " + pkg.Name)
						buildError(pkgs, err, dir)
						timeoutHit = true
						output.Close()
						timer.Stop()
						os.RemoveAll(dir)
						return
					}
				}
				fmt.Println("Build succeeded for " + pkg.Name)
				for _, pkg2 := range pkgs {
					pkg2.Status = packages.Uptodate
					pkg2.LastBuildStatus = packages.Built
					pkg2.Version = buildVersion
					packages.UpdatePackage(pkg2, true)
				}
				os.RemoveAll(dir)
				output.Close()
				timer.Stop()
				return
			default:
				time.Sleep(time.Second * 10)
			}
		}
	}()

	io.Copy(io.Discard, output.Reader)
	output.Close()
	timer.Stop()
	if timeoutHit {
		return nil
	}

	if !checkBuild(pkgs, pkg, dir) {
		fmt.Println("No build output for " + pkg.Name)
		buildError(pkgs, err, dir)
		return nil
	} else {
		fmt.Println("Build succeeded for " + pkg.Name)
		for _, pkg2 := range pkgs {
			pkg2.Status = packages.Uptodate
			pkg2.LastBuildStatus = packages.Built
			pkg2.Version = buildVersion
			packages.UpdatePackage(pkg2, true)
		}
	}
	os.RemoveAll(dir)
	return nil
}

func buildError(pkgs []packages.PackageInfo, err error, dir string) {
	os.RemoveAll(dir)
	if err != nil {
		slog.Error(err.Error())
	}
	for _, pkg2 := range pkgs {
		pkg2.LastBuildStatus = packages.Error
		pkg2.BuildAttempts++
		packages.UpdatePackage(pkg2, true)
	}
}

func checkBuild(pkgs []packages.PackageInfo, pkg packages.PackageInfo, dir string) bool {
	// Check if there is a build
	buildErr := true
	entries, err := os.ReadDir(dir)
	if err != nil {
		buildError(pkgs, err, dir)
		return true
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "dbgsym") || strings.Contains(entry.Name(), "source") {
			os.Remove(dir + "/" + entry.Name())
			continue
		}
		if filepath.Ext(entry.Name()) == ".log" {
			cmd := exec.Command("/bin/sh", "-c", "mv "+dir+"/"+entry.Name()+" /srv/www/buildlogs/"+pkg.Name+"_"+entry.Name())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			err = cmd.Run()
			if err != nil {
				slog.Error(err.Error())
			}
			cmd = exec.Command("/bin/sh", "-c", "chmod 777 "+"/srv/www/buildlogs/"+pkg.Name+"_"+entry.Name())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			err = cmd.Run()
			if err != nil {
				slog.Error(err.Error())
			}
		}
		if filepath.Ext(entry.Name()) == ".deb" {
			cmd := exec.Command("/bin/sh", "-c", "mv "+dir+"/"+entry.Name()+" "+config.Configs.DeboutputDir+entry.Name())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			err = cmd.Run()
			if err != nil {
				slog.Error(err.Error())
				continue
			}
			cmd = exec.Command("/bin/sh", "-c", "chmod 777 "+config.Configs.DeboutputDir+entry.Name())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			err = cmd.Run()
			if err != nil {
				slog.Error(err.Error())
				continue
			}
			buildErr = false
			continue
		}
	}
	return !buildErr
}
