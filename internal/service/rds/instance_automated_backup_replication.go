package rds

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceInstanceAutomatedBackupReplication() *schema.Resource {
	return &schema.Resource{
		Create: resourceInstanceAutomatedBackupReplicationCreate,
		Read:   resourceInstanceAutomatedBackupReplicationRead,
		Delete: resourceInstanceAutomatedBackupReplicationDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"kms_key_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidARN,
			},
			"retention_period": {
				Type:     schema.TypeInt,
				ForceNew: true,
				Optional: true,
				Default:  7,
			},
			"source_db_instance_arn": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidARN,
			},
		},
	}
}

func resourceInstanceAutomatedBackupReplicationCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).RDSConn

	input := &rds.StartDBInstanceAutomatedBackupsReplicationInput{
		BackupRetentionPeriod: aws.Int64(int64(d.Get("retention_period").(int))),
		SourceDBInstanceArn:   aws.String(d.Get("source_db_instance_arn").(string)),
	}

	if v, ok := d.GetOk("kms_key_id"); ok {
		input.KmsKeyId = aws.String(v.(string))
	}

	log.Printf("[DEBUG] Starting RDS instance automated backup replication: %s", input)
	output, err := conn.StartDBInstanceAutomatedBackupsReplication(input)

	if err != nil {
		return fmt.Errorf("error starting RDS instance automated backup replication: %w", err)
	}

	d.SetId(aws.StringValue(output.DBInstanceAutomatedBackup.DBInstanceAutomatedBackupsArn))

	if _, err := waitDBInstanceAutomatedBackupCreated(conn, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return fmt.Errorf("error waiting for DB instance automated backup (%s) create: %w", d.Id(), err)
	}

	return resourceInstanceAutomatedBackupReplicationRead(d, meta)
}

func resourceInstanceAutomatedBackupReplicationRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).RDSConn

	backup, err := FindDBInstanceAutomatedBackupByARN(conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] RDS instance automated backup replication %s not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading RDS instance automated backup replication (%s): %w", d.Id(), err)
	}

	d.Set("kms_key_id", backup.KmsKeyId)
	d.Set("retention_period", backup.BackupRetentionPeriod)
	d.Set("source_db_instance_arn", backup.DBInstanceArn)

	return nil
}

func resourceInstanceAutomatedBackupReplicationDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).RDSConn

	backup, err := FindDBInstanceAutomatedBackupByARN(conn, d.Id())

	if tfresource.NotFound(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading RDS instance automated backup replication (%s): %w", d.Id(), err)
	}

	dbInstanceID := aws.StringValue(backup.DBInstanceIdentifier)
	sourceDatabaseARN, err := arn.Parse(aws.StringValue(backup.DBInstanceArn))

	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Stopping RDS instance automated backup replication: %s", d.Id())
	_, err = conn.StopDBInstanceAutomatedBackupsReplication(&rds.StopDBInstanceAutomatedBackupsReplicationInput{
		SourceDBInstanceArn: aws.String(d.Get("source_db_instance_arn").(string)),
	})

	if err != nil {
		return fmt.Errorf("error stopping RDS instance automated backup replication (%s): %w", d.Id(), err)
	}

	// Create a new client to the source region.
	sourceDatabaseConn := conn
	if sourceDatabaseARN.Region != meta.(*conns.AWSClient).Region {
		sourceDatabaseConn = rds.New(meta.(*conns.AWSClient).Session, aws.NewConfig().WithRegion(sourceDatabaseARN.Region))
	}

	if _, err := waitDBInstanceAutomatedBackupDeleted(sourceDatabaseConn, dbInstanceID, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return fmt.Errorf("error waiting for DB instance automated backup (%s) delete: %w", d.Id(), err)
	}

	return nil
}
