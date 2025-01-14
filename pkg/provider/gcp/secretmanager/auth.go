/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package secretmanager

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	"github.com/external-secrets/external-secrets/pkg/provider/gcp/workloadidentity"
	"github.com/external-secrets/external-secrets/pkg/utils/resolvers"
)

func NewTokenSource(ctx context.Context, auth esv1beta1.GCPSMAuth, projectID, storeKind string, kube kclient.Client, namespace string) (oauth2.TokenSource, error) {
	ts, err := serviceAccountTokenSource(ctx, auth, storeKind, kube, namespace)
	if ts != nil || err != nil {
		return ts, err
	}

	if auth.WorkloadIdentity == nil {
		return google.DefaultTokenSource(ctx, CloudPlatformRole)
	}

	saKey := types.NamespacedName{
		Name:      auth.WorkloadIdentity.ServiceAccountRef.Name,
		Namespace: namespace,
	}

	// only ClusterStore is allowed to set namespace (and then it's required)
	if isClusterKind && auth.WorkloadIdentity.ServiceAccountRef.Namespace != nil {
		saKey.Namespace = *auth.WorkloadIdentity.ServiceAccountRef.Namespace
	}

	idp := workloadidentity.ClusterIdentityProvider(auth.WorkloadIdentity.ClusterName, auth.WorkloadIdentity.ClusterLocation)
	if auth.WorkloadIdentity.ClusterMembershipName != "" {
		idp = workloadidentity.FleetIdentityProvider(auth.WorkloadIdentity.ClusterMembershipName)
	}

	wip, err := workloadidentity.NewProvider(ctx, projectID, idp)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize workload identity: %w", err)
	}
	defer wip.Close()

	ts, err = wip.TokenSource(ctx, kube, saKey, auth.WorkloadIdentity.ServiceAccountRef.Audiences...)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func serviceAccountTokenSource(ctx context.Context, auth esv1beta1.GCPSMAuth, storeKind string, kube kclient.Client, namespace string) (oauth2.TokenSource, error) {
	sr := auth.SecretRef
	if sr == nil {
		return nil, nil
	}
	credentials, err := resolvers.SecretKeyRef(
		ctx,
		kube,
		storeKind,
		namespace,
		&auth.SecretRef.SecretAccessKey)
	if err != nil {
		return nil, err
	}
	config, err := google.JWTConfigFromJSON([]byte(credentials), CloudPlatformRole)
	if err != nil {
		return nil, fmt.Errorf(errUnableProcessJSONCredentials, err)
	}
	return config.TokenSource(ctx), nil
}
