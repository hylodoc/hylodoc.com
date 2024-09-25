package email

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"strings"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/config"
)

const (
	newPostUpdateTemplate = `View this post at: {{ .BlogLink }}

{{ .BlogBody }}

Unsubscribe {{ .UnsubscribeLink }}`

	/* <protocol>://<blog_subdomain_name>.<service_name>/blogs/<blog_id>/unsubscribe
	 *	e.g. <http>://<localhost.com:7999>/blogs/<1>/unsubscribe
	 *	e.g. <https>://<progstack.com>/blogs/<596>/unsubscribe
	 */
	unsubscribeLinkTemplate = "%s://%s.%s/blogs/%d/unsubscribe"
)

type NewPostUpdateParams struct {
	Blog       BlogParams
	Subscriber SubscriberParams
	Post       PostParams
}

type SubscriberParams struct {
	To               string /* subscriber email address */
	UnsubscribeToken string /* subscribers unsubscribe token */
}

type BlogParams struct {
	ID        int32
	From      string /* email sender address (configured on blog (repository) by user)*/
	Subdomain string
}

type PostParams struct {
	Link    string /* link to blog post on hosted website */
	Body    string /* body of post */
	Subject string /* title of blog post */
}

func SendNewPostUpdate(client *resend.Client, params NewPostUpdateParams) error {
	log.Println("sending newPostUpdate email...")

	from := params.Blog.From
	to := params.Subscriber.To
	subject := params.Post.Subject
	text, err := newPostUpdateText(params)
	if err != nil {
		return fmt.Errorf("error building newPostUpdateText: %w", err)
	}

	request := &resend.SendEmailRequest{
		From:    from,
		To:      []string{to},
		Subject: subject,
		Text:    text,
	}

	sent, err := client.Emails.SendWithContext(context.TODO(), request)
	if err != nil {
		log.Printf("email response: %v", sent)
		return fmt.Errorf("error sending email to `%s: %w", to, err)
	}
	return nil
}

func newPostUpdateText(params NewPostUpdateParams) (string, error) {
	log.Println("parsing newPostUpdateTemplate...")

	tmpl, err := template.New("email").Parse(newPostUpdateTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing newPostUpdateTemplate: %w", err)
	}
	var b strings.Builder

	unsubscribeLink, err := buildUnsubscribeLink(params.Blog.ID, params.Blog.Subdomain, params.Subscriber.UnsubscribeToken)
	if err != nil {
		return "", fmt.Errorf("error building unsubscribe link: %w", err)
	}

	err = tmpl.Execute(&b, struct {
		BlogLink        string
		BlogBody        string
		UnsubscribeLink string
	}{
		BlogLink:        params.Post.Link,
		BlogBody:        params.Post.Body,
		UnsubscribeLink: unsubscribeLink,
	})
	if err != nil {
		return "", fmt.Errorf("error executing newPostUpdateTemplate: %w", err)
	}
	return b.String(), nil
}

func buildUnsubscribeLink(blogID int32, blogSubdomain, unsubscribeToken string) (string, error) {
	log.Println("building unsubscribe link...")

	/* build base url */
	base := fmt.Sprintf(
		unsubscribeLinkTemplate,
		config.Config.Progstack.Protocol,
		blogSubdomain,
		config.Config.Progstack.ServiceName,
		blogID,
	)
	log.Printf("unsubscribe link base: %s\n", base)

	/* add token as query parameter */
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Add("token", unsubscribeToken)
	u.RawQuery = params.Encode()

	link := u.String()
	log.Printf("unsubscribe link: %s\n", link)

	return link, nil
}
