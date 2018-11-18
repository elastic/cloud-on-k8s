package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewInitContainers(t *testing.T) {
	type args struct {
		imageName        string
		linkedFiles      LinkedFilesArray
		SetVMMaxMapCount bool
		keystoreSettings KeystoreInit
	}
	tests := []struct {
		name                       string
		args                       args
		expectedNumberOfContainers int
	}{
		{
			name: "with SetVMMaxMapCount enabled",
			args: args{
				imageName:        "image",
				linkedFiles:      LinkedFilesArray{},
				SetVMMaxMapCount: true,
			},
			expectedNumberOfContainers: 2,
		},
		{
			name: "with SetVMMaxMapCount disabled",
			args: args{
				imageName:        "image",
				linkedFiles:      LinkedFilesArray{},
				SetVMMaxMapCount: false,
			},
			expectedNumberOfContainers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers, err := NewInitContainers(tt.args.imageName, tt.args.linkedFiles, tt.args.keystoreSettings, tt.args.SetVMMaxMapCount)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNumberOfContainers, len(containers))
		})
	}
}
