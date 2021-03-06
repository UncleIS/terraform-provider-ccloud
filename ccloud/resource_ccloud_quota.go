package ccloud

import (
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/sapcc/gophercloud-limes/resources/v1/projects"
	"github.com/sapcc/limes"
)

var (
	SERVICES = map[string]map[string]limes.Unit{
		"compute": {
			"cores":     limes.UnitNone,
			"instances": limes.UnitNone,
			"ram":       limes.UnitMebibytes,
		},
		"volumev2": {
			"capacity":  limes.UnitGibibytes,
			"snapshots": limes.UnitNone,
			"volumes":   limes.UnitNone,
		},
		"network": {
			"floating_ips":         limes.UnitNone,
			"networks":             limes.UnitNone,
			"ports":                limes.UnitNone,
			"rbac_policies":        limes.UnitNone,
			"routers":              limes.UnitNone,
			"security_group_rules": limes.UnitNone,
			"security_groups":      limes.UnitNone,
			"subnet_pools":         limes.UnitNone,
			"subnets":              limes.UnitNone,
			"healthmonitors":       limes.UnitNone,
			"l7policies":           limes.UnitNone,
			"listeners":            limes.UnitNone,
			"loadbalancers":        limes.UnitNone,
			"pools":                limes.UnitNone,
		},
		"dns": {
			"zones":      limes.UnitNone,
			"recordsets": limes.UnitNone,
		},
		"sharev2": {
			"share_networks":    limes.UnitNone,
			"share_capacity":    limes.UnitGibibytes,
			"shares":            limes.UnitNone,
			"snapshot_capacity": limes.UnitGibibytes,
			"share_snapshots":   limes.UnitNone,
		},
		"object-store": {
			"capacity": limes.UnitBytes,
		},
	}
)

func resourceCCloudQuota() *schema.Resource {
	quotaResource := &schema.Resource{
		SchemaVersion: 1,

		Read:   resourceCCloudQuotaRead,
		Update: resourceCCloudQuotaCreateOrUpdate,
		Create: resourceCCloudQuotaCreateOrUpdate,
		Delete: resourceCCloudQuotaDelete,

		Schema: map[string]*schema.Schema{
			"domain_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"project_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}

	for service, resources := range SERVICES {
		elem := &schema.Resource{
			Schema: make(map[string]*schema.Schema, len(resources)),
		}

		for resource := range resources {
			elem.Schema[resource] = &schema.Schema{
				Type:     schema.TypeInt,
				Required: false,
				Optional: true,
			}
		}

		quotaResource.Schema[sanitize(service)] = &schema.Schema{
			Type:     schema.TypeList,
			Optional: true,
			Elem:     elem,
			MaxItems: 1,
		}
	}

	return quotaResource
}

func resourceCCloudQuotaRead(d *schema.ResourceData, meta interface{}) error {
	domainID := d.Get("domain_id").(string)
	projectID := d.Get("project_id").(string)

	log.Printf("[QUOTA] Reading Quota for: %s/%s", domainID, projectID)

	config := meta.(*Config)
	limes, err := config.limesV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack limes client: %s", err)
	}

	quota, err := projects.Get(limes, domainID, projectID, projects.GetOpts{}).Extract()
	if err != nil {
		return fmt.Errorf("Error getting Limes project: %s", err)
	}

	d.SetId(projectID)
	for service, resources := range SERVICES {
		for resource := range resources {
			key := fmt.Sprintf("%s.%s", sanitize(service), resource)
			if quota.Services[service] == nil || quota.Services[service].Resources[resource] == nil {
				continue
			}
			d.Set(key, quota.Services[service].Resources[resource].Quota)
			log.Printf("[QUOTA] %s.%s: %s", service, resource, toString(quota.Services[service].Resources[resource]))
		}
	}

	return nil
}

func resourceCCloudQuotaCreateOrUpdate(d *schema.ResourceData, meta interface{}) error {
	domainID := d.Get("domain_id").(string)
	projectID := d.Get("project_id").(string)
	services := limes.QuotaRequest{}

	log.Printf("[QUOTA] Updating Quota for: %s/%s", domainID, projectID)

	config := meta.(*Config)
	client, err := config.limesV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack limes client: %s", err)
	}

	for _service, resources := range SERVICES {
		service := sanitize(_service)
		if _, ok := d.GetOk(service); ok && d.HasChange(service) {
			log.Printf("[QUOTA] Service Changed: %s", service)

			quota := limes.ServiceQuotaRequest{}
			for resource, unit := range resources {
				key := fmt.Sprintf("%s.0.%s", service, resource)

				if v, ok := d.GetOk(key); ok && d.HasChange(key) {
					log.Printf("[QUOTA] Resource Changed: %s", key)
					quota[resource] = limes.ValueWithUnit{uint64(v.(int)), unit}
					log.Printf("[QUOTA] %s.%s: %s", service, resource, quota[resource].String())
				}
			}
			services[_service] = quota
		}
	}

	opts := projects.UpdateOpts{Services: services}
	_, err = projects.Update(client, domainID, projectID, opts)
	if err != nil {
		return fmt.Errorf("Error updating Limes project: %s", err)
	}

	log.Printf("[QUOTA] Resulting Quota for: %s/%s", domainID, projectID)

	d.SetId(projectID)
	resourceCCloudQuotaRead(d, meta)

	return nil
}

func resourceCCloudQuotaDelete(d *schema.ResourceData, meta interface{}) error {
	d.SetId("")
	return nil
}

func toString(r *limes.ProjectResourceReport) string {
	return limes.ValueWithUnit{r.Quota, r.Unit}.String()
}

func sanitize(s string) string {
	return strings.Replace(s, "-", "", -1)
}
