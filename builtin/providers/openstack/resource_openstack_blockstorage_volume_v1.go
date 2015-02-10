package openstack

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/racker/perigee"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack/blockstorage/v1/volumes"
)

func resourceBlockStorageVolumeV1() *schema.Resource {
	return &schema.Resource{
		Create: resourceBlockStorageVolumeV1Create,
		Read:   resourceBlockStorageVolumeV1Read,
		Update: resourceBlockStorageVolumeV1Update,
		Delete: resourceBlockStorageVolumeV1Delete,

		Schema: map[string]*schema.Schema{
			"region": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				DefaultFunc: envDefaultFunc("OS_REGION_NAME"),
			},
			"size": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"metadata": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: false,
			},
			"snapshot_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"source_vol_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"image_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"volume_type": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceBlockStorageVolumeV1Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV1Client(d.Get("region").(string))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack block storage client: %s", err)
	}

	createOpts := &volumes.CreateOpts{
		Description: d.Get("description").(string),
		Name:        d.Get("name").(string),
		Size:        d.Get("size").(int),
		SnapshotID:  d.Get("snapshot_id").(string),
		SourceVolID: d.Get("source_vol_id").(string),
		ImageID:     d.Get("image_id").(string),
		VolumeType:  d.Get("volume_type").(string),
		Metadata:    resourceContainerMetadataV2(d),
	}

	log.Printf("[INFO] Requesting volume creation")
	v, err := volumes.Create(blockStorageClient, createOpts).Extract()
	if err != nil {
		return fmt.Errorf("Error creating OpenStack volume: %s", err)
	}
	log.Printf("[INFO] Volume ID: %s", v.ID)

	// Store the ID now
	d.SetId(v.ID)

	// Wait for the volume to become available.
	log.Printf(
		"[DEBUG] Waiting for volume (%s) to become available",
		v.ID)

	stateConf := &resource.StateChangeConf{
		Target:     "available",
		Refresh:    VolumeV1StateRefreshFunc(blockStorageClient, v.ID),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for volume (%s) to become ready: %s",
			v.ID, err)
	}

	return resourceBlockStorageVolumeV1Read(d, meta)
}

func resourceBlockStorageVolumeV1Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV1Client(d.Get("region").(string))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack block storage client: %s", err)
	}

	v, err := volumes.Get(blockStorageClient, d.Id()).Extract()
	if err != nil {
		return CheckDeleted(d, err, "volume")
	}

	log.Printf("\n\ngot volume: %+v\n\n", v)

	log.Printf("[DEBUG] Retreived volume %s: %+v", d.Id(), v)

	d.Set("region", d.Get("region").(string))
	d.Set("size", v.Size)

	if t, exists := d.GetOk("description"); exists && t != "" {
		d.Set("description", v.Description)
	} else {
		d.Set("description", "")
	}

	if t, exists := d.GetOk("name"); exists && t != "" {
		d.Set("name", v.Name)
	} else {
		d.Set("name", "")
	}

	if t, exists := d.GetOk("snapshot_id"); exists && t != "" {
		d.Set("snapshot_id", v.SnapshotID)
	} else {
		d.Set("snapshot_id", "")
	}

	if t, exists := d.GetOk("source_vol_id"); exists && t != "" {
		d.Set("source_vol_id", v.SourceVolID)
	} else {
		d.Set("source_vol_id", "")
	}

	if t, exists := d.GetOk("volume_type"); exists && t != "" {
		d.Set("volume_type", v.VolumeType)
	} else {
		d.Set("volume_type", "")
	}

	if t, exists := d.GetOk("metadata"); exists && t != "" {
		d.Set("metadata", v.Metadata)
	} else {
		d.Set("metadata", "")
	}

	return nil
}

func resourceBlockStorageVolumeV1Update(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV1Client(d.Get("region").(string))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack block storage client: %s", err)
	}

	updateOpts := volumes.UpdateOpts{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	}

	if d.HasChange("metadata") {
		updateOpts.Metadata = resourceVolumeMetadataV1(d)
	}

	_, err = volumes.Update(blockStorageClient, d.Id(), updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("Error updating OpenStack volume: %s", err)
	}

	return resourceBlockStorageVolumeV1Read(d, meta)
}

func resourceBlockStorageVolumeV1Delete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV1Client(d.Get("region").(string))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack block storage client: %s", err)
	}

	err = volumes.Delete(blockStorageClient, d.Id()).ExtractErr()
	if err != nil {
		return fmt.Errorf("Error deleting OpenStack volume: %s", err)
	}

	// Wait for the volume to delete before moving on.
	log.Printf("[DEBUG] Waiting for volume (%s) to delete", d.Id())

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"deleting"},
		Target:     "deleted",
		Refresh:    VolumeV1StateRefreshFunc(blockStorageClient, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for volume (%s) to delete: %s",
			d.Id(), err)
	}

	d.SetId("")
	return nil
}

func resourceVolumeMetadataV1(d *schema.ResourceData) map[string]string {
	m := make(map[string]string)
	for key, val := range d.Get("metadata").(map[string]interface{}) {
		m[key] = val.(string)
	}
	return m
}

// VolumeV1StateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// an OpenStack volume.
func VolumeV1StateRefreshFunc(client *gophercloud.ServiceClient, volumeID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		v, err := volumes.Get(client, volumeID).Extract()
		if err != nil {
			errCode, ok := err.(*perigee.UnexpectedResponseCodeError)
			if !ok {
				return nil, "", err
			}
			if errCode.Actual == 404 {
				return v, "deleted", nil
			}
			return nil, "", err
		}

		return v, v.Status, nil
	}
}
