package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/FleexSecurity/fleex/pkg/sshutils"
	"github.com/FleexSecurity/fleex/pkg/utils"
	"github.com/FleexSecurity/fleex/provider"
	"github.com/digitalocean/godo"
	"github.com/spf13/viper"
)

type DigitaloceanService struct{}

// SpawnFleet spawns a DigitalOcean fleet
func (d DigitaloceanService) SpawnFleet(fleetName string, fleetCount int, image string, region string, size string, sshFingerprint string, tags []string, token string) {
	existingFleet := d.GetFleet(fleetName, token)

	client := godo.NewFromToken(token)
	ctx := context.TODO()
	digitaloceanPasswd := viper.GetString("digitalocean.password")
	if digitaloceanPasswd == "" {
		digitaloceanPasswd = "1337rootPass"
	}

	droplets := []string{}

	for i := 0; i < fleetCount; i++ {
		droplets = append(droplets, fleetName+"-"+strconv.Itoa(i+1+len(existingFleet)))
	}

	user_data := `#!/bin/bash
sudo sed -i "/^[^#]*PasswordAuthentication[[:space:]]no/c\PasswordAuthentication yes" /etc/ssh/sshd_config
sudo service sshd restart
echo 'op:` + digitaloceanPasswd + `' | sudo chpasswd`

	var createRequest *godo.DropletMultiCreateRequest
	imageIntID, err := strconv.Atoi(image)
	if err != nil {
		createRequest = &godo.DropletMultiCreateRequest{
			Names:  droplets,
			Region: region,
			Size:   size,
			// UserData: "echo 'root:" + digitaloceanPasswd + "' | chpasswd",
			UserData: user_data,
			Image: godo.DropletCreateImage{
				Slug: image,
			},
			SSHKeys: []godo.DropletCreateSSHKey{
				{Fingerprint: sshFingerprint},
			},
			Tags: tags,
		}
	} else {
		createRequest = &godo.DropletMultiCreateRequest{
			Names:    droplets,
			Region:   region,
			Size:     size,
			UserData: user_data,
			Image: godo.DropletCreateImage{
				ID: imageIntID,
			},
			SSHKeys: []godo.DropletCreateSSHKey{
				{Fingerprint: sshFingerprint},
			},
			Tags: tags,
		}
	}

	_, _, err = client.Droplets.CreateMultiple(ctx, createRequest)

	if err != nil {
		utils.Log.Fatal(err)
	}

}

func (d DigitaloceanService) GetFleet(fleetName, token string) (fleet []provider.Box) {
	boxes := d.GetBoxes(token)

	for _, box := range boxes {
		if strings.HasPrefix(box.Label, fleetName) {
			fleet = append(fleet, box)
		}
	}
	return fleet
}

// GetBox returns a single box by its label
func (d DigitaloceanService) GetBox(boxName, token string) provider.Box {
	boxes := d.GetBoxes(token)

	for _, box := range boxes {
		if box.Label == boxName {
			return box
		}
	}
	utils.Log.Fatal("Box not found!")
	return provider.Box{}
}

func (d DigitaloceanService) GetBoxes(token string) (boxes []provider.Box) {
	client := godo.NewFromToken(token)
	ctx := context.TODO()
	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 9999,
	}

	droplets, _, err := client.Droplets.List(ctx, opt)
	if err != nil {
		utils.Log.Fatal(err)
	}

	for _, d := range droplets {
		ip, _ := d.PublicIPv4()
		dID := strconv.Itoa(d.ID)
		boxes = append(boxes, provider.Box{ID: dID, Label: d.Name, Group: "", Status: d.Status, IP: ip})
	}
	return boxes
}

func (d DigitaloceanService) ListBoxes(token string) {
	boxes := d.GetBoxes(token)
	for _, box := range boxes {
		fmt.Println(box.ID, box.Label, box.Group, box.Status, box.IP)
	}
}

func (d DigitaloceanService) DeleteFleet(name string, token string) {
	droplets := d.GetBoxes(token)
	for _, droplet := range droplets {
		if droplet.Label == name {
			// It's a single box
			d.DeleteBoxByID(droplet.ID, token)
			return
		}
	}

	// Otherwise, we got a fleet to delete
	for _, droplet := range droplets {
		if strings.HasPrefix(droplet.Label, name) {
			d.DeleteBoxByID(droplet.ID, token)
		}
	}
}

func (l DigitaloceanService) GetImages(token string) (images []provider.Image) {
	return
}

func (d DigitaloceanService) ListImages(token string) {
	client := godo.NewFromToken(token)
	ctx := context.TODO()
	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 9999,
	}

	images, _, err := client.Images.ListUser(ctx, opt)
	if err != nil {
		utils.Log.Fatal(err)
	}
	for _, image := range images {
		fmt.Println(image.ID, image.Name, image.Status, image.SizeGigaBytes)
	}
}

func (d DigitaloceanService) DeleteBoxByID(ID string, token string) {
	client := godo.NewFromToken(token)
	ctx := context.TODO()

	ID1, _ := strconv.Atoi(ID)
	_, err := client.Droplets.Delete(ctx, ID1)
	if err != nil {
		utils.Log.Fatal(err)
	}
}

func (l DigitaloceanService) DeleteBoxByLabel(label string, token string) {
	linodes := l.GetBoxes(token)
	for _, linode := range linodes {
		if linode.Label == label && linode.Label != "BugBountyUbuntu" {
			l.DeleteBoxByID(linode.ID, token)
		}
	}
}

func (d DigitaloceanService) CountFleet(fleetName string, boxes []provider.Box) (count int) {
	for _, box := range boxes {
		if strings.HasPrefix(box.Label, fleetName) {
			count++
		}
	}
	return count
}

func (d DigitaloceanService) RunCommand(name, command string, port int, username, password, token string) {
	//doSshUser := viper.GetString("digitalocean.username")
	//doSshPort := viper.GetInt("digitalocean.port")
	// doSshPassword := viper.GetString("digitalocean.password")
	boxes := d.GetBoxes(token)

	// fmt.Println(port, username, password)

	for _, box := range boxes {
		if box.Label == name {
			// It's a single box
			boxIP := box.IP
			sshutils.RunCommand(command, boxIP, port, username, password)
			return
		}
	}

	// Otherwise, send command to a fleet
	fleetSize := d.CountFleet(name, boxes)

	fleet := make(chan *provider.Box, fleetSize)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(fleetSize)

	for i := 0; i < fleetSize; i++ {
		go func() {
			for {
				box := <-fleet

				if box == nil {
					break
				}
				boxIP := box.IP
				sshutils.RunCommand(command, boxIP, port, username, password)
			}
			processGroup.Done()
		}()
	}

	for i := range boxes {
		if strings.HasPrefix(boxes[i].Label, name) {
			fleet <- &boxes[i]
		}
	}

	close(fleet)
	processGroup.Wait()
}

func (d DigitaloceanService) CreateImage(token string, diskID int, label string) {
	client := godo.NewFromToken(token)
	ctx := context.TODO()

	_, _, err := client.DropletActions.Snapshot(ctx, diskID, label)
	if err != nil {
		utils.Log.Fatal(err)
	}
}