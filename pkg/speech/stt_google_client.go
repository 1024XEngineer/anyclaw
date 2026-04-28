package speech

import (
	"context"

	speechapi "cloud.google.com/go/speech/apiv1"
	speechpb "cloud.google.com/go/speech/apiv1/speechpb"
	"google.golang.org/api/option"
)

type googleRecognizeAPI interface {
	Recognize(context.Context, *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error)
	Close() error
}

type googleRecognizeClient struct {
	client *speechapi.Client
}

func newGoogleRecognizeClient(ctx context.Context, clientOpts ...option.ClientOption) (googleRecognizeAPI, error) {
	client, err := speechapi.NewRESTClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}
	return &googleRecognizeClient{client: client}, nil
}

func (c *googleRecognizeClient) Recognize(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
	return c.client.Recognize(ctx, req)
}

func (c *googleRecognizeClient) Close() error {
	return c.client.Close()
}
