package discovery

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ensureFileStorageCompatibilityNodeLabel(t *testing.T) {
	type args struct {
		node       *corev1.Node
		compatible bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "do not crash if labels is nil",
			args: args{
				node:       &corev1.Node{},
				compatible: false,
			},
			want: false,
		},
		{
			name: "add label",
			args: args{
				node:       &corev1.Node{},
				compatible: true,
			},
			want: true,
		},
		{
			name: "remove label",
			args: args{
				node: &corev1.Node{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							FileStorageCompatibilityNodeLabel: TrueLabelValue,
						},
					},
				},
				compatible: false,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureFileStorageCompatibilityNodeLabel(tt.args.node, tt.args.compatible); got != tt.want {
				t.Errorf("ensureFileStorageCompatibilityNodeLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}
