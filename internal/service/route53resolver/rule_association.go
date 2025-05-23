// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package route53resolver

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"
	awstypes "github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/enum"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKResource("aws_route53_resolver_rule_association", name="Rule Association")
func resourceRuleAssociation() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceRuleAssociationCreate,
		ReadWithoutTimeout:   resourceRuleAssociationRead,
		DeleteWithoutTimeout: resourceRuleAssociationDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			names.AttrName: {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validResolverName,
			},
			"resolver_rule_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 64),
			},
			names.AttrVPCID: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 64),
			},
		},
	}
}

func resourceRuleAssociationCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).Route53ResolverClient(ctx)

	input := &route53resolver.AssociateResolverRuleInput{
		ResolverRuleId: aws.String(d.Get("resolver_rule_id").(string)),
		VPCId:          aws.String(d.Get(names.AttrVPCID).(string)),
	}

	if v, ok := d.GetOk(names.AttrName); ok {
		input.Name = aws.String(v.(string))
	}

	output, err := conn.AssociateResolverRule(ctx, input)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating Route53 Resolver Rule Association: %s", err)
	}

	d.SetId(aws.ToString(output.ResolverRuleAssociation.Id))

	if _, err := waitRuleAssociationCreated(ctx, conn, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for Route53 Resolver Rule Association (%s) create: %s", d.Id(), err)
	}

	return append(diags, resourceRuleAssociationRead(ctx, d, meta)...)
}

func resourceRuleAssociationRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).Route53ResolverClient(ctx)

	ruleAssociation, err := findResolverRuleAssociationByID(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] Route53 Resolver Rule Association (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading Route53 Resolver Rule Association (%s): %s", d.Id(), err)
	}

	d.Set(names.AttrName, ruleAssociation.Name)
	d.Set("resolver_rule_id", ruleAssociation.ResolverRuleId)
	d.Set(names.AttrVPCID, ruleAssociation.VPCId)

	return diags
}

func resourceRuleAssociationDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).Route53ResolverClient(ctx)

	log.Printf("[DEBUG] Deleting Route53 Resolver Rule Association: %s", d.Id())
	_, err := conn.DisassociateResolverRule(ctx, &route53resolver.DisassociateResolverRuleInput{
		ResolverRuleId: aws.String(d.Get("resolver_rule_id").(string)),
		VPCId:          aws.String(d.Get(names.AttrVPCID).(string)),
	})

	if errs.IsA[*awstypes.ResourceNotFoundException](err) {
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting Route53 Resolver Rule Association (%s): %s", d.Id(), err)
	}

	if _, err := waitRuleAssociationDeleted(ctx, conn, d.Id(), d.Timeout(schema.TimeoutDelete)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for Route53 Resolver Rule Association (%s) delete: %s", d.Id(), err)
	}

	return diags
}

func findResolverRuleAssociationByID(ctx context.Context, conn *route53resolver.Client, id string) (*awstypes.ResolverRuleAssociation, error) {
	input := &route53resolver.GetResolverRuleAssociationInput{
		ResolverRuleAssociationId: aws.String(id),
	}

	output, err := conn.GetResolverRuleAssociation(ctx, input)

	if errs.IsA[*awstypes.ResourceNotFoundException](err) {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: input,
		}
	}

	if err != nil {
		return nil, err
	}

	if output == nil || output.ResolverRuleAssociation == nil {
		return nil, tfresource.NewEmptyResultError(input)
	}

	return output.ResolverRuleAssociation, nil
}

func statusRuleAssociation(ctx context.Context, conn *route53resolver.Client, id string) retry.StateRefreshFunc {
	return func() (any, string, error) {
		output, err := findResolverRuleAssociationByID(ctx, conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, string(output.Status), nil
	}
}

func waitRuleAssociationCreated(ctx context.Context, conn *route53resolver.Client, id string, timeout time.Duration) (*awstypes.ResolverRuleAssociation, error) {
	stateConf := &retry.StateChangeConf{
		Pending:    enum.Slice(awstypes.ResolverRuleAssociationStatusCreating),
		Target:     enum.Slice(awstypes.ResolverRuleAssociationStatusComplete),
		Refresh:    statusRuleAssociation(ctx, conn, id),
		Timeout:    timeout,
		Delay:      10 * time.Second,
		MinTimeout: 5 * time.Second,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*awstypes.ResolverRuleAssociation); ok {
		tfresource.SetLastError(err, errors.New(aws.ToString(output.StatusMessage)))

		return output, err
	}

	return nil, err
}

func waitRuleAssociationDeleted(ctx context.Context, conn *route53resolver.Client, id string, timeout time.Duration) (*awstypes.ResolverRuleAssociation, error) {
	stateConf := &retry.StateChangeConf{
		Pending:    enum.Slice(awstypes.ResolverRuleAssociationStatusDeleting),
		Target:     []string{},
		Refresh:    statusRuleAssociation(ctx, conn, id),
		Timeout:    timeout,
		Delay:      10 * time.Second,
		MinTimeout: 5 * time.Second,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*awstypes.ResolverRuleAssociation); ok {
		tfresource.SetLastError(err, errors.New(aws.ToString(output.StatusMessage)))

		return output, err
	}

	return nil, err
}
