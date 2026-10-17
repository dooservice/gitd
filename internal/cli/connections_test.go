package cli

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestAConnectionReadsAsItsKindPrefixAndWhomItActsAs(t *testing.T) {
	var out bytes.Buffer
	connectionLine(&out, &gitdv1.Connection{Id: "login-1", Kind: gitdv1.Connection_KIND_GITHUB_LOGIN,
		UrlPrefix: "https://github.com", ActsAs: "josue", Expires: timestamppb.New(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC))})
	connectionLine(&out, &gitdv1.Connection{Id: "file-1", Kind: gitdv1.Connection_KIND_TOKEN, UrlPrefix: "https://github.com/acme", FromFile: true})
	want := "login-1  github_login  https://github.com  as josue  until 2027-04-01\nfile-1  token  https://github.com/acme  (gitd.toml)\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
