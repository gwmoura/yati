// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/mgo.v2/bson"
)

var errAmbiguousContainer error = errors.New("ambiguous container name")

func (p *dockerProvisioner) GetContainer(id string) (*container.Container, error) {
	var containers []container.Container
	coll := p.Collection()
	defer coll.Close()
	pattern := fmt.Sprintf("^%s.*", id)
	err := coll.Find(bson.M{"id": bson.RegEx{Pattern: pattern}}).All(&containers)
	if err != nil {
		return nil, err
	}
	lenContainers := len(containers)
	if lenContainers == 0 {
		return nil, &provision.UnitNotFoundError{ID: id}
	}
	if lenContainers > 1 {
		return nil, errAmbiguousContainer
	}
	return &containers[0], nil
}

func (p *dockerProvisioner) GetContainerByName(name string) (*container.Container, error) {
	var containers []container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(bson.M{"name": name}).All(&containers)
	if err != nil {
		return nil, err
	}
	lenContainers := len(containers)
	if lenContainers == 0 {
		return nil, &provision.UnitNotFoundError{ID: name}
	}
	if lenContainers > 1 {
		return nil, errAmbiguousContainer
	}
	return &containers[0], nil
}

func (p *dockerProvisioner) listContainersByHost(address string) ([]container.Container, error) {
	return p.ListContainers(bson.M{"hostaddr": address})
}

func (p *dockerProvisioner) listRunningContainersByHost(address string) ([]container.Container, error) {
	return p.ListContainers(bson.M{
		"hostaddr": address,
		"status": bson.M{
			"$nin": []string{
				provision.StatusCreated.String(),
				provision.StatusBuilding.String(),
				provision.StatusStopped.String(),
			},
		},
	})
}

func (p *dockerProvisioner) listContainersByProcess(appName, processName string) ([]container.Container, error) {
	query := bson.M{"appname": appName}
	if processName != "" {
		query["processname"] = processName
	}
	return p.ListContainers(query)
}

func (p *dockerProvisioner) listContainersByApp(appName string) ([]container.Container, error) {
	return p.ListContainers(bson.M{"appname": appName})
}

func (p *dockerProvisioner) listContainersByAppAndHost(appNames, addresses []string) ([]container.Container, error) {
	query := bson.M{}
	if len(appNames) > 0 {
		query["appname"] = bson.M{"$in": appNames}
	}
	if len(addresses) > 0 {
		query["hostaddr"] = bson.M{"$in": addresses}
	}
	return p.ListContainers(query)
}

func (p *dockerProvisioner) listRunnableContainersByApp(appName string) ([]container.Container, error) {
	return p.ListContainers(bson.M{
		"appname": appName,
		"status": bson.M{
			"$nin": []string{
				provision.StatusCreated.String(),
				provision.StatusBuilding.String(),
				provision.StatusStopped.String(),
			},
		},
	})
}

func (p *dockerProvisioner) listAllContainers() ([]container.Container, error) {
	return p.ListContainers(nil)
}

func (p *dockerProvisioner) listAppsForNodes(nodes []*cluster.Node) ([]string, error) {
	coll := p.Collection()
	defer coll.Close()
	nodeNames := make([]string, len(nodes))
	for i, n := range nodes {
		nodeNames[i] = net.URLToHost(n.Address)
	}
	var appNames []string
	err := coll.Find(bson.M{"hostaddr": bson.M{"$in": nodeNames}}).Distinct("appname", &appNames)
	return appNames, err
}

func (p *dockerProvisioner) ListContainers(query bson.M) ([]container.Container, error) {
	var list []container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(query).All(&list)
	return list, err
}

func (p *dockerProvisioner) updateContainers(query bson.M, update bson.M) error {
	coll := p.Collection()
	defer coll.Close()
	_, err := coll.UpdateAll(query, update)
	return err
}

func (p *dockerProvisioner) getOneContainerByAppName(appName string) (*container.Container, error) {
	var c container.Container
	coll := p.Collection()
	defer coll.Close()
	err := coll.Find(bson.M{"appname": appName}).One(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *dockerProvisioner) getContainerCountForAppName(appName string) (int, error) {
	coll := p.Collection()
	defer coll.Close()
	return coll.Find(bson.M{"appname": appName}).Count()
}

func (p *dockerProvisioner) listUnresponsiveContainers(maxUnresponsiveTime time.Duration) ([]container.Container, error) {
	now := time.Now().UTC()
	return p.ListContainers(bson.M{
		"lastsuccessstatusupdate": bson.M{"$lt": now.Add(-maxUnresponsiveTime)},
		"hostport":                bson.M{"$ne": ""},
		"status":                  bson.M{"$ne": provision.StatusStopped.String()},
	})
}
