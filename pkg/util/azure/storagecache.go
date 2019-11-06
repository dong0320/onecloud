// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package azure

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud/pkg/cloudprovider"
	"yunion.io/x/onecloud/pkg/compute/options"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	"yunion.io/x/onecloud/pkg/mcclient/modules"
	"yunion.io/x/onecloud/pkg/util/fileutils2"
	"yunion.io/x/onecloud/pkg/util/qemuimg"
)

const (
	DefaultStorageAccount string = "image"
	DefaultContainer      string = "image-cache"

	DefaultReadBlockSize int64 = 4 * 1024 * 1024
)

type SStoragecache struct {
	region *SRegion

	iimages []cloudprovider.ICloudImage
}

func (self *SStoragecache) GetMetadata() *jsonutils.JSONDict {
	return nil
}

func (self *SStoragecache) GetId() string {
	return fmt.Sprintf("%s-%s", self.region.client.providerId, self.region.GetId())
}

func (self *SStoragecache) GetName() string {
	return fmt.Sprintf("%s-%s", self.region.client.providerName, self.region.GetId())
}

func (self *SStoragecache) GetStatus() string {
	return "available"
}

func (self *SStoragecache) Refresh() error {
	return nil
}

func (self *SStoragecache) GetGlobalId() string {
	return fmt.Sprintf("%s-%s", self.region.client.providerId, self.region.GetGlobalId())
}

func (self *SStoragecache) IsEmulated() bool {
	return false
}

func (self *SStoragecache) fetchImages() error {
	if images, err := self.region.GetImages(""); err != nil {
		return err
	} else {
		self.iimages = make([]cloudprovider.ICloudImage, len(images))
		for i := 0; i < len(images); i++ {
			images[i].storageCache = self
			self.iimages[i] = &images[i]
		}
	}
	return nil
}

func (self *SStoragecache) GetIImages() ([]cloudprovider.ICloudImage, error) {
	if self.iimages == nil {
		if err := self.fetchImages(); err != nil {
			return nil, err
		}
	}
	return self.iimages, nil
}

func (self *SStoragecache) GetIImageById(extId string) (cloudprovider.ICloudImage, error) {
	img, err := self.region.GetImageById(extId)
	if err != nil {
		return nil, err
	}
	img.storageCache = self
	return &img, nil
}

func (self *SStoragecache) GetPath() string {
	return ""
}

func (self *SStoragecache) UploadImage(ctx context.Context, userCred mcclient.TokenCredential, image *cloudprovider.SImageCreateOption, isForce bool) (string, error) {
	if len(image.ExternalId) > 0 {
		log.Debugf("UploadImage: Image external ID exists %s", image.ExternalId)
		status, err := self.region.GetImageStatus(image.ExternalId)
		if err != nil {
			log.Errorf("GetImageStatus error %s", err)
		}
		if status == ImageStatusAvailable && !isForce {
			return image.ExternalId, nil
		}
	} else {
		log.Debugf("UploadImage: no external ID")
	}
	return self.uploadImage(ctx, userCred, image, isForce, options.Options.TempPath)
}

func (self *SStoragecache) checkStorageAccount() (*SStorageAccount, error) {
	storageaccounts, err := self.region.GetStorageAccounts()
	if err != nil {
		return nil, errors.Wrap(err, "GetStorageAccounts")
	}
	if len(storageaccounts) == 0 {
		storageaccount, err := self.region.CreateStorageAccount(self.region.Name)
		if err != nil {
			return nil, errors.Wrap(err, "CreateStorageAccount")
		}
		return storageaccount, nil
	}
	for i := 0; i < len(storageaccounts); i++ {
		if id, ok := storageaccounts[i].Tags["id"]; ok && id == self.region.Name {
			return &storageaccounts[i], nil
		}
	}

	storageaccount := storageaccounts[0]
	if storageaccount.Tags == nil {
		storageaccount.Tags = map[string]string{}
	}
	storageaccount.Tags["id"] = self.region.Name
	err = self.region.client.Update(jsonutils.Marshal(storageaccount), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "Update(%s)", jsonutils.Marshal(storageaccount).String())
	}
	return &storageaccount, nil
}

func (self *SStoragecache) uploadImage(ctx context.Context, userCred mcclient.TokenCredential, image *cloudprovider.SImageCreateOption, isForce bool, tmpPath string) (string, error) {
	s := auth.GetAdminSession(ctx, options.Options.Region, "")
	meta, reader, err := modules.Images.Download(s, image.ImageId, string(qemuimg.VHD), false)
	if err != nil {
		return "", err
	}
	// {
	// 	"checksum":"d0ab0450979977c6ada8d85066a6e484",
	// 	"container_format":"bare",
	// 	"created_at":"2018-08-10T04:18:07",
	// 	"deleted":"False",
	// 	"disk_format":"vhd",
	// 	"id":"64189033-3ad4-413c-b074-6bf0b6be8508",
	// 	"is_public":"False",
	// 	"min_disk":"0",
	// 	"min_ram":"0",
	// 	"name":"centos-7.3.1611-20180104.vhd",
	// 	"owner":"5124d80475434da8b41fee48d5be94df",
	// 	"properties":{
	// 		"os_arch":"x86_64",
	// 		"os_distribution":"CentOS",
	// 		"os_type":"Linux",
	// 		"os_version":"7.3.1611-VHD"
	// 	},
	// 	"protected":"False",
	// 	"size":"2028505088",
	// 	"status":"active",
	// 	"updated_at":"2018-08-10T04:20:59"
	// }
	log.Infof("meta data %s", meta)

	imageNameOnBlob, _ := meta.GetString("name")
	if !strings.HasSuffix(imageNameOnBlob, ".vhd") {
		imageNameOnBlob = fmt.Sprintf("%s.vhd", imageNameOnBlob)
	}
	tmpFile := fmt.Sprintf("%s/%s", tmpPath, imageNameOnBlob)
	defer os.Remove(tmpFile)
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", errors.Wrap(err, "os.Create(tmpFile)")
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return "", errors.Wrap(err, "io.Copy(f, reader)")
	}

	storageaccount, err := self.checkStorageAccount()
	if err != nil {
		return "", errors.Wrap(err, "self.checkStorageAccount")
	}

	blobURI, err := storageaccount.UploadFile("image-cache", tmpFile)
	if err != nil {
		return "", errors.Wrap(err, "storageaccount.UploadFile")
	}

	size, _ := meta.Int("size")

	img, err := self.region.CreateImageByBlob(image.ImageId, image.OsType, blobURI, int32(size>>30))
	if err != nil {
		return "", errors.Wrap(err, "CreateImageByBlob")
	}
	return img.GetGlobalId(), nil
}

func (self *SStoragecache) CreateIImage(snapshotId, imageName, osType, imageDesc string) (cloudprovider.ICloudImage, error) {
	if image, err := self.region.CreateImage(snapshotId, imageName, osType, imageDesc); err != nil {
		return nil, err
	} else {
		image.storageCache = self
		return image, nil
	}
}

func (self *SStoragecache) DownloadImage(userCred mcclient.TokenCredential, imageId string, extId string, path string) (jsonutils.JSONObject, error) {
	return self.downloadImage(userCred, imageId, extId, path)
}

func (self *SStoragecache) downloadImage(userCred mcclient.TokenCredential, imageId string, extId string, path string) (jsonutils.JSONObject, error) {
	// TODO: need to fix scenarios where image is a public image
	// XXX Qiu Jian
	if image, err := self.region.getPrivateImage(extId); err != nil {
		return nil, err
	} else if snapshotId := image.Properties.StorageProfile.OsDisk.Snapshot.ID; len(snapshotId) == 0 {
		return nil, cloudprovider.ErrNotFound
	} else if uri, err := self.region.GrantAccessSnapshot(snapshotId); err != nil {
		return nil, err
	} else if resp, err := http.Get(uri); err != nil {
		return nil, err
	} else {
		tmpImageFile, err := ioutil.TempFile(path, "temp")
		if err != nil {
			return nil, err
		}
		defer tmpImageFile.Close()
		defer os.Remove(tmpImageFile.Name())
		{
			sf := fileutils2.NewSparseFileWriter(tmpImageFile)
			data := make([]byte, DefaultReadBlockSize)
			written := int64(0)
			for {
				n, err := resp.Body.Read(data)
				if n > 0 {
					if n, err := sf.Write(data); err != nil {
						return nil, err
					} else {
						written += int64(n)
					}
				} else if err == io.EOF {
					if written <= resp.ContentLength {
						return nil, fmt.Errorf("got eof: expecting %d bytes, got %d", resp.ContentLength, written)
					}
					break
				} else {
					return nil, err
				}
			}
			if err := sf.PreClose(); err != nil {
				return nil, err
			}
		}

		if _, err := tmpImageFile.Seek(0, os.SEEK_SET); err != nil {
			return nil, errors.Wrap(err, "seek")
		}
		s := auth.GetAdminSession(context.Background(), options.Options.Region, "")
		params := jsonutils.Marshal(map[string]string{"image_id": imageId, "disk-format": "raw"})
		if result, err := modules.Images.Upload(s, params, tmpImageFile, resp.ContentLength); err != nil {
			return nil, err
		} else {
			return result, nil
		}

	}
}

func (region *SRegion) GetIStoragecaches() ([]cloudprovider.ICloudStoragecache, error) {
	storageCache := region.getStoragecache()
	return []cloudprovider.ICloudStoragecache{storageCache}, nil
}
