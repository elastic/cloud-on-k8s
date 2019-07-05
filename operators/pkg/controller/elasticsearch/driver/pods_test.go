// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

//
//func Test_newPVCFromTemplate(t *testing.T) {
//	type args struct {
//		claimTemplate corev1.PersistentVolumeClaim
//		pod           *corev1.Pod
//	}
//	tests := []struct {
//		name string
//		args args
//		want *corev1.PersistentVolumeClaim
//	}{
//		{
//			name: "Create a simple PVC from a template and a pod",
//			args: args{
//				claimTemplate: corev1.PersistentVolumeClaim{
//					ObjectMeta: metav1.ObjectMeta{
//						Name: esvolume.ElasticsearchDataVolumeName,
//					},
//				},
//				pod: &corev1.Pod{
//					ObjectMeta: metav1.ObjectMeta{
//						Name: "elasticsearch-sample-es-6bw9qkw77k",
//						Labels: map[string]string{
//							"l1":                                   "v1",
//							"l2":                                   "v2",
//							common.TypeLabelName:                   "elasticsearch",
//							label.ClusterNameLabelName:             "cluster-name",
//							string(label.NodeTypesMasterLabelName): "true",
//							string(label.NodeTypesMLLabelName):     "true",
//							string(label.NodeTypesIngestLabelName): "true",
//							string(label.NodeTypesDataLabelName):   "true",
//							label.VersionLabelName:                 "7.1.0",
//						},
//					},
//				},
//			},
//			want: &corev1.PersistentVolumeClaim{
//				ObjectMeta: metav1.ObjectMeta{
//					Name: "elasticsearch-sample-es-6bw9qkw77k-" + esvolume.ElasticsearchDataVolumeName,
//					Labels: map[string]string{
//						// only a subset of labels should be copied over the pvc
//						common.TypeLabelName:                   "elasticsearch",
//						label.ClusterNameLabelName:             "cluster-name",
//						string(label.NodeTypesMasterLabelName): "true",
//						string(label.NodeTypesMLLabelName):     "true",
//						string(label.NodeTypesIngestLabelName): "true",
//						string(label.NodeTypesDataLabelName):   "true",
//						label.VersionLabelName:                 "7.1.0",
//						// additional pod name label should be there
//						label.PodNameLabelName:    "elasticsearch-sample-es-6bw9qkw77k",
//						label.VolumeNameLabelName: volume.ElasticsearchDataVolumeName,
//					},
//				},
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if diff := deep.Equal(newPVCFromTemplate(tt.args.claimTemplate, tt.args.pod), tt.want); diff != nil {
//				t.Error(diff)
//			}
//		})
//	}
//}
//
//func Test_createElasticsearchPod(t *testing.T) {
//	client := k8s.WrapClient(fake.NewFakeClient())
//	podSpecCtx := pod.PodSpecContext{
//		PodTemplate: corev1.PodTemplateSpec{
//			Spec: corev1.PodSpec{
//				Containers: []corev1.Container{{Name: "foo"}},
//			},
//		},
//		NodeSpec: v1alpha1.NodeSpec{
//			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
//				{
//					ObjectMeta: metav1.ObjectMeta{
//						Name: esvolume.ElasticsearchDataVolumeName,
//					},
//					Spec: corev1.PersistentVolumeClaimSpec{},
//				},
//			},
//		},
//	}
//	es := v1alpha1.Elasticsearch{}
//	pod := corev1.Pod{
//		ObjectMeta: metav1.ObjectMeta{
//			Namespace: "ns",
//			Name:      "name",
//			Labels: map[string]string{
//				"a": "b",
//			},
//		},
//		Spec: corev1.PodSpec{
//			Volumes: []corev1.Volume{
//				{
//					Name: esvolume.TransportCertificatesSecretVolumeName,
//					VolumeSource: corev1.VolumeSource{
//						Secret: &corev1.SecretVolumeSource{
//							SecretName: "should-be-replaced",
//						},
//					},
//				},
//				{
//					Name: settings.ConfigVolumeName,
//					VolumeSource: corev1.VolumeSource{
//						Secret: &corev1.SecretVolumeSource{
//							SecretName: "should-be-replaced",
//						},
//					},
//				},
//				{
//					Name:         esvolume.ElasticsearchDataVolumeName,
//					VolumeSource: corev1.VolumeSource{},
//				},
//			},
//		},
//	}
//	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
//	err := createElasticsearchPod(client, scheme.Scheme, es, reconcile.NewState(es), pod, podSpecCtx, &pvcutils.OrphanedPersistentVolumeClaims{})
//	require.NoError(t, err)
//
//	err = client.Get(k8s.ExtractNamespacedName(&pod), &pod)
//	require.NoError(t, err)
//
//	// should have a volume for config (existing one replaced)
//	found := false
//	configSecretVolumeName := settings.ConfigSecretVolume(pod.Name).Name()
//	for _, v := range pod.Spec.Volumes {
//		if v.Name == configSecretVolumeName {
//			require.NotEqual(t, "should-be-replaced", v.Secret.SecretName)
//			found = true
//		}
//	}
//	require.True(t, found)
//	// should have a PVC assigned (volume replaced)
//	found = false
//	pvcName := ""
//	for _, v := range pod.Spec.Volumes {
//		if v.Name == esvolume.ElasticsearchDataVolumeName {
//			pvcName = v.PersistentVolumeClaim.ClaimName
//			require.NotEmpty(t, pvcName)
//			found = true
//		}
//	}
//	require.True(t, found)
//	// PVC should be created
//	var pvc corev1.PersistentVolumeClaim
//	require.NoError(t, client.Get(types.NamespacedName{Namespace: "ns", Name: pvcName}, &pvc))
//}
