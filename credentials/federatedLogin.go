package credentials

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/credentials"
	"github.com/awslabs/aws-sdk-go/service/iam"
	"github.com/awslabs/aws-sdk-go/service/sts"
)

const (
	IAMPolicy = `{"Version": "2012-10-17", "Statement": [{"Effect": "Allow", "Action": ["*"], "Resource": ["*"]}]}`
)

func (a *AWSCredentialStore) GetConsoleLoginURL(env string, timeout int) (string, error) {
	e, ok := a.Credentials[env]
	if !ok {
		return "", fmt.Errorf("Environment '%s' was not found.", env)
	}

	c := credentials.NewStaticCredentials(e.AWSAccessKeyID, e.AWSSecretAccessKey, "")

	// Get the username of the current user
	iam := iam.New(&aws.Config{Credentials: c})
	usr, err := iam.GetUser(nil)
	if err != nil {
		return "", err
	}

	username := "root"
	if usr.User.UserName != nil {
		username = *usr.User.UserName
	}

	// Create STS url for current user
	svc := sts.New(&aws.Config{Credentials: c})

	resp, err := svc.GetFederationToken(&sts.GetFederationTokenInput{
		Name:            aws.String(fmt.Sprintf("awsenv-%s", username)),
		DurationSeconds: aws.Long(int64(timeout)),
		Policy:          aws.String(IAMPolicy),
	})

	if err != nil {
		return "", err
	}

	signinToken, err := a.getFederatedSigninToken(resp)
	if err != nil {
		return "", err
	}

	p := url.Values{
		"Action":      []string{"login"},
		"Issuer":      []string{"https://github.com/Luzifer/awsenv"},
		"Destination": []string{fmt.Sprintf("https://console.aws.amazon.com/console/home?region=%s", e.AWSRegion)},
		"SigninToken": []string{signinToken},
	}
	out := url.URL{
		Scheme:   "https",
		Host:     "signin.aws.amazon.com",
		Path:     "federation",
		RawQuery: p.Encode(),
	}

	return out.String(), nil

}

func (a *AWSCredentialStore) getFederatedSigninToken(token *sts.GetFederationTokenOutput) (string, error) {
	tsc, _ := json.Marshal(struct {
		SessionID    string `json:"sessionId"`
		SessionKey   string `json:"sessionKey"`
		SessionToken string `json:"sessionToken"`
	}{
		SessionID:    *token.Credentials.AccessKeyID,
		SessionKey:   *token.Credentials.SecretAccessKey,
		SessionToken: *token.Credentials.SessionToken,
	})

	p := url.Values{
		"Action":  []string{"getSigninToken"},
		"Session": []string{string(tsc)},
	}
	u := url.URL{
		Scheme:   "https",
		Host:     "signin.aws.amazon.com",
		Path:     "federation",
		RawQuery: p.Encode(),
	}
	req, _ := http.NewRequest("GET", u.String(), nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	sit := struct {
		SigninToken string
	}{}
	json.NewDecoder(res.Body).Decode(&sit)

	return sit.SigninToken, nil
}
