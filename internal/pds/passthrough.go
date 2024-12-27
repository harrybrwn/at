package pds

import (
	"net/http"

	"github.com/harrybrwn/at/api/app/bsky"
	chatbsky "github.com/harrybrwn/at/api/chat/bsky"
	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/api/tools/ozone"
	"github.com/harrybrwn/at/xrpc"
)

type ComAtProtoPassthrough struct {
	*atproto.AdminClient
	*atproto.IdentityClient
	*atproto.LabelClient
	*atproto.ModerationClient
	*atproto.RepoClient
	*atproto.ServerClient
	*atproto.SyncClient
	*atproto.TempClient
}

func NewComAtProtoPassthrough(c *xrpc.Client) *ComAtProtoPassthrough {
	return &ComAtProtoPassthrough{
		AdminClient:      atproto.NewAdminClient(c),
		IdentityClient:   atproto.NewIdentityClient(c),
		LabelClient:      atproto.NewLabelClient(c),
		ModerationClient: atproto.NewModerationClient(c),
		RepoClient:       atproto.NewRepoClient(c),
		ServerClient:     atproto.NewServerClient(c),
		SyncClient:       atproto.NewSyncClient(c),
		TempClient:       atproto.NewTempClient(c),
	}
}

func (pt *ComAtProtoPassthrough) Apply(srv *xrpc.Server, middleware ...func(http.Handler) http.Handler) {
	srv.AddHandlers(
		atproto.NewAdminDeleteAccountHandler(pt.AdminClient),
		atproto.NewAdminDisableAccountInvitesHandler(pt.AdminClient),
		atproto.NewAdminDisableInviteCodesHandler(pt.AdminClient),
		atproto.NewAdminEnableAccountInvitesHandler(pt.AdminClient),
		atproto.NewAdminGetAccountInfoHandler(pt.AdminClient),
		atproto.NewAdminGetAccountInfosHandler(pt.AdminClient),
		atproto.NewAdminGetInviteCodesHandler(pt.AdminClient),
		atproto.NewAdminGetSubjectStatusHandler(pt.AdminClient),
		atproto.NewAdminSearchAccountsHandler(pt.AdminClient),
		atproto.NewAdminSendEmailHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountEmailHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountHandleHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountPasswordHandler(pt.AdminClient),
		atproto.NewAdminUpdateSubjectStatusHandler(pt.AdminClient),
		atproto.NewIdentityGetRecommendedDidCredentialsHandler(pt.IdentityClient),
		atproto.NewIdentityRequestPlcOperationSignatureHandler(pt.IdentityClient),
		atproto.NewIdentityResolveHandleHandler(pt.IdentityClient),
		atproto.NewIdentitySignPlcOperationHandler(pt.IdentityClient),
		atproto.NewIdentitySubmitPlcOperationHandler(pt.IdentityClient),
		atproto.NewIdentityUpdateHandleHandler(pt.IdentityClient),
		atproto.NewLabelQueryLabelsHandler(pt.LabelClient),
		atproto.NewLabelSubscribeLabelsHandler(pt.LabelClient),
		atproto.NewModerationCreateReportHandler(pt.ModerationClient),
		atproto.NewRepoApplyWritesHandler(pt.RepoClient),
		atproto.NewRepoCreateRecordHandler(pt.RepoClient),
		atproto.NewRepoDeleteRecordHandler(pt.RepoClient),
		atproto.NewRepoDescribeRepoHandler(pt.RepoClient),
		atproto.NewRepoGetRecordHandler(pt.RepoClient),
		atproto.NewRepoImportRepoHandler(pt.RepoClient),
		atproto.NewRepoListMissingBlobsHandler(pt.RepoClient),
		atproto.NewRepoListRecordsHandler(pt.RepoClient),
		atproto.NewRepoUploadBlobHandler(pt.RepoClient),
		atproto.NewServerActivateAccountHandler(pt.ServerClient),
		atproto.NewServerCheckAccountStatusHandler(pt.ServerClient),
		atproto.NewServerConfirmEmailHandler(pt.ServerClient),
		atproto.NewServerCreateAccountHandler(pt.ServerClient),
		atproto.NewServerCreateAppPasswordHandler(pt.ServerClient),
		atproto.NewServerCreateInviteCodeHandler(pt.ServerClient),
		atproto.NewServerCreateInviteCodesHandler(pt.ServerClient),
		atproto.NewServerCreateSessionHandler(pt.ServerClient),
		atproto.NewServerDeactivateAccountHandler(pt.ServerClient),
		atproto.NewServerDeleteAccountHandler(pt.ServerClient),
		atproto.NewServerDeleteSessionHandler(pt.ServerClient),
		atproto.NewServerDescribeServerHandler(pt.ServerClient),
		atproto.NewServerGetAccountInviteCodesHandler(pt.ServerClient),
		atproto.NewServerGetServiceAuthHandler(pt.ServerClient),
		atproto.NewServerGetSessionHandler(pt.ServerClient),
		atproto.NewServerListAppPasswordsHandler(pt.ServerClient),
		atproto.NewServerRefreshSessionHandler(pt.ServerClient),
		atproto.NewServerRequestAccountDeleteHandler(pt.ServerClient),
		atproto.NewServerRequestEmailConfirmationHandler(pt.ServerClient),
		atproto.NewServerRequestEmailUpdateHandler(pt.ServerClient),
		atproto.NewServerRequestPasswordResetHandler(pt.ServerClient),
		atproto.NewServerReserveSigningKeyHandler(pt.ServerClient),
		atproto.NewServerResetPasswordHandler(pt.ServerClient),
		atproto.NewServerRevokeAppPasswordHandler(pt.ServerClient),
		atproto.NewServerUpdateEmailHandler(pt.ServerClient),
		atproto.NewSyncGetBlobHandler(pt.SyncClient),
		atproto.NewSyncGetBlocksHandler(pt.SyncClient),
		atproto.NewSyncGetCheckoutHandler(pt.SyncClient),
		atproto.NewSyncGetHeadHandler(pt.SyncClient),
		atproto.NewSyncGetLatestCommitHandler(pt.SyncClient),
		atproto.NewSyncGetRecordHandler(pt.SyncClient),
		atproto.NewSyncGetRepoHandler(pt.SyncClient),
		atproto.NewSyncGetRepoStatusHandler(pt.SyncClient),
		atproto.NewSyncListBlobsHandler(pt.SyncClient),
		atproto.NewSyncListReposHandler(pt.SyncClient),
		atproto.NewSyncNotifyOfUpdateHandler(pt.SyncClient),
		atproto.NewSyncRequestCrawlHandler(pt.SyncClient),
		atproto.NewSyncSubscribeReposHandler(pt.SyncClient),
		atproto.NewTempAddReservedHandleHandler(pt.TempClient),
		atproto.NewTempCheckSignupQueueHandler(pt.TempClient),
		atproto.NewTempFetchLabelsHandler(pt.TempClient),
		atproto.NewTempRequestPhoneVerificationHandler(pt.TempClient),
	)
}

type AppBskyPassthrough struct {
	*bsky.ActorClient
	*bsky.FeedClient
	*bsky.GraphClient
	*bsky.LabelerClient
	*bsky.NotificationClient
	*bsky.UnspeccedClient
	*bsky.VideoClient
}

func NewAppBskyPassthrough(c *xrpc.Client) *AppBskyPassthrough {
	return &AppBskyPassthrough{
		ActorClient:        bsky.NewActorClient(c),
		FeedClient:         bsky.NewFeedClient(c),
		GraphClient:        bsky.NewGraphClient(c),
		LabelerClient:      bsky.NewLabelerClient(c),
		NotificationClient: bsky.NewNotificationClient(c),
		UnspeccedClient:    bsky.NewUnspeccedClient(c),
		VideoClient:        bsky.NewVideoClient(c),
	}
}

func (pt *AppBskyPassthrough) Apply(srv *xrpc.Server, middleware ...func(http.Handler) http.Handler) {
	srv.AddHandlers(
		bsky.NewActorGetPreferencesHandler(pt.ActorClient),
		bsky.NewActorGetProfileHandler(pt.ActorClient),
		bsky.NewActorGetProfilesHandler(pt.ActorClient),
		bsky.NewActorGetSuggestionsHandler(pt.ActorClient),
		bsky.NewActorPutPreferencesHandler(pt.ActorClient),
		bsky.NewActorSearchActorsTypeaheadHandler(pt.ActorClient),
		bsky.NewFeedDescribeFeedGeneratorHandler(pt.FeedClient),
		bsky.NewFeedGetActorFeedsHandler(pt.FeedClient),
		bsky.NewFeedGetActorLikesHandler(pt.FeedClient),
		bsky.NewFeedGetAuthorFeedHandler(pt.FeedClient),
		bsky.NewFeedGetFeedGeneratorHandler(pt.FeedClient),
		bsky.NewFeedGetFeedGeneratorsHandler(pt.FeedClient),
		bsky.NewFeedGetFeedHandler(pt.FeedClient),
		bsky.NewFeedGetFeedSkeletonHandler(pt.FeedClient),
		bsky.NewFeedGetLikesHandler(pt.FeedClient),
		bsky.NewFeedGetListFeedHandler(pt.FeedClient),
		bsky.NewFeedGetPostThreadHandler(pt.FeedClient),
		bsky.NewFeedGetPostsHandler(pt.FeedClient),
		bsky.NewFeedGetQuotesHandler(pt.FeedClient),
		bsky.NewFeedGetRepostedByHandler(pt.FeedClient),
		bsky.NewFeedGetSuggestedFeedsHandler(pt.FeedClient),
		bsky.NewFeedGetTimelineHandler(pt.FeedClient),
		bsky.NewFeedSearchPostsHandler(pt.FeedClient),
		bsky.NewFeedSendInteractionsHandler(pt.FeedClient),
		bsky.NewGraphGetActorStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphGetBlocksHandler(pt.GraphClient),
		bsky.NewGraphGetFollowersHandler(pt.GraphClient),
		bsky.NewGraphGetFollowsHandler(pt.GraphClient),
		bsky.NewGraphGetKnownFollowersHandler(pt.GraphClient),
		bsky.NewGraphGetListBlocksHandler(pt.GraphClient),
		bsky.NewGraphGetListHandler(pt.GraphClient),
		bsky.NewGraphGetListMutesHandler(pt.GraphClient),
		bsky.NewGraphGetListsHandler(pt.GraphClient),
		bsky.NewGraphGetMutesHandler(pt.GraphClient),
		bsky.NewGraphGetStarterPackHandler(pt.GraphClient),
		bsky.NewGraphGetStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphGetSuggestedFollowsByActorHandler(pt.GraphClient),
		bsky.NewGraphMuteActorHandler(pt.GraphClient),
		bsky.NewGraphMuteActorListHandler(pt.GraphClient),
		bsky.NewGraphMuteThreadHandler(pt.GraphClient),
		bsky.NewGraphSearchStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphUnmuteActorHandler(pt.GraphClient),
		bsky.NewGraphUnmuteActorListHandler(pt.GraphClient),
		bsky.NewGraphUnmuteThreadHandler(pt.GraphClient),
		bsky.NewLabelerGetServicesHandler(pt.LabelerClient),
		bsky.NewNotificationGetUnreadCountHandler(pt.NotificationClient),
		bsky.NewNotificationListNotificationsHandler(pt.NotificationClient),
		bsky.NewNotificationPutPreferencesHandler(pt.NotificationClient),
		bsky.NewNotificationRegisterPushHandler(pt.NotificationClient),
		bsky.NewNotificationUpdateSeenHandler(pt.NotificationClient),
		bsky.NewUnspeccedGetConfigHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetPopularFeedGeneratorsHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetSuggestionsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetTaggedSuggestionsHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchActorsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchPostsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchStarterPacksSkeletonHandler(pt.UnspeccedClient),
		bsky.NewVideoGetJobStatusHandler(pt.VideoClient),
		bsky.NewVideoGetUploadLimitsHandler(pt.VideoClient),
		bsky.NewVideoUploadVideoHandler(pt.VideoClient),
	)
}

type Passthrough struct {
	*atproto.AdminClient
	*atproto.IdentityClient
	*atproto.LabelClient
	*atproto.ModerationClient
	*atproto.RepoClient
	*atproto.ServerClient
	*atproto.SyncClient
	*atproto.TempClient
	*bsky.ActorClient
	*bsky.FeedClient
	*bsky.GraphClient
	*bsky.LabelerClient
	*bsky.NotificationClient
	*bsky.UnspeccedClient
	*bsky.VideoClient

	AppBsky    *AppBskyPassthrough
	ComAtProto *ComAtProtoPassthrough
}

func NewPassthrough(c *xrpc.Client) *Passthrough {
	return &Passthrough{
		ActorClient:        bsky.NewActorClient(c),
		AdminClient:        atproto.NewAdminClient(c),
		FeedClient:         bsky.NewFeedClient(c),
		GraphClient:        bsky.NewGraphClient(c),
		IdentityClient:     atproto.NewIdentityClient(c),
		LabelClient:        atproto.NewLabelClient(c),
		LabelerClient:      bsky.NewLabelerClient(c),
		ModerationClient:   atproto.NewModerationClient(c),
		NotificationClient: bsky.NewNotificationClient(c),
		RepoClient:         atproto.NewRepoClient(c),
		ServerClient:       atproto.NewServerClient(c),
		SyncClient:         atproto.NewSyncClient(c),
		TempClient:         atproto.NewTempClient(c),
		UnspeccedClient:    bsky.NewUnspeccedClient(c),
		VideoClient:        bsky.NewVideoClient(c),
	}
}

func (pt *Passthrough) Apply(srv *xrpc.Server, middleware ...func(http.Handler) http.Handler) {
	srv.AddHandlers(
		atproto.NewAdminDeleteAccountHandler(pt.AdminClient),
		atproto.NewAdminDisableAccountInvitesHandler(pt.AdminClient),
		atproto.NewAdminDisableInviteCodesHandler(pt.AdminClient),
		atproto.NewAdminEnableAccountInvitesHandler(pt.AdminClient),
		atproto.NewAdminGetAccountInfoHandler(pt.AdminClient),
		atproto.NewAdminGetAccountInfosHandler(pt.AdminClient),
		atproto.NewAdminGetInviteCodesHandler(pt.AdminClient),
		atproto.NewAdminGetSubjectStatusHandler(pt.AdminClient),
		atproto.NewAdminSearchAccountsHandler(pt.AdminClient),
		atproto.NewAdminSendEmailHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountEmailHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountHandleHandler(pt.AdminClient),
		atproto.NewAdminUpdateAccountPasswordHandler(pt.AdminClient),
		atproto.NewAdminUpdateSubjectStatusHandler(pt.AdminClient),
		atproto.NewIdentityGetRecommendedDidCredentialsHandler(pt.IdentityClient),
		atproto.NewIdentityRequestPlcOperationSignatureHandler(pt.IdentityClient),
		atproto.NewIdentityResolveHandleHandler(pt.IdentityClient),
		atproto.NewIdentitySignPlcOperationHandler(pt.IdentityClient),
		atproto.NewIdentitySubmitPlcOperationHandler(pt.IdentityClient),
		atproto.NewIdentityUpdateHandleHandler(pt.IdentityClient),
		atproto.NewLabelQueryLabelsHandler(pt.LabelClient),
		atproto.NewLabelSubscribeLabelsHandler(pt.LabelClient),
		atproto.NewModerationCreateReportHandler(pt.ModerationClient),
		atproto.NewRepoApplyWritesHandler(pt.RepoClient),
		atproto.NewRepoCreateRecordHandler(pt.RepoClient),
		atproto.NewRepoDeleteRecordHandler(pt.RepoClient),
		atproto.NewRepoDescribeRepoHandler(pt.RepoClient),
		atproto.NewRepoGetRecordHandler(pt.RepoClient),
		atproto.NewRepoImportRepoHandler(pt.RepoClient),
		atproto.NewRepoListMissingBlobsHandler(pt.RepoClient),
		atproto.NewRepoListRecordsHandler(pt.RepoClient),
		atproto.NewRepoUploadBlobHandler(pt.RepoClient),
		atproto.NewServerActivateAccountHandler(pt.ServerClient),
		atproto.NewServerCheckAccountStatusHandler(pt.ServerClient),
		atproto.NewServerConfirmEmailHandler(pt.ServerClient),
		atproto.NewServerCreateAccountHandler(pt.ServerClient),
		atproto.NewServerCreateAppPasswordHandler(pt.ServerClient),
		atproto.NewServerCreateInviteCodeHandler(pt.ServerClient),
		atproto.NewServerCreateInviteCodesHandler(pt.ServerClient),
		atproto.NewServerCreateSessionHandler(pt.ServerClient),
		atproto.NewServerDeactivateAccountHandler(pt.ServerClient),
		atproto.NewServerDeleteAccountHandler(pt.ServerClient),
		atproto.NewServerDeleteSessionHandler(pt.ServerClient),
		atproto.NewServerDescribeServerHandler(pt.ServerClient),
		atproto.NewServerGetAccountInviteCodesHandler(pt.ServerClient),
		atproto.NewServerGetServiceAuthHandler(pt.ServerClient),
		atproto.NewServerGetSessionHandler(pt.ServerClient),
		atproto.NewServerListAppPasswordsHandler(pt.ServerClient),
		atproto.NewServerRefreshSessionHandler(pt.ServerClient),
		atproto.NewServerRequestAccountDeleteHandler(pt.ServerClient),
		atproto.NewServerRequestEmailConfirmationHandler(pt.ServerClient),
		atproto.NewServerRequestEmailUpdateHandler(pt.ServerClient),
		atproto.NewServerRequestPasswordResetHandler(pt.ServerClient),
		atproto.NewServerReserveSigningKeyHandler(pt.ServerClient),
		atproto.NewServerResetPasswordHandler(pt.ServerClient),
		atproto.NewServerRevokeAppPasswordHandler(pt.ServerClient),
		atproto.NewServerUpdateEmailHandler(pt.ServerClient),
		atproto.NewSyncGetBlobHandler(pt.SyncClient),
		atproto.NewSyncGetBlocksHandler(pt.SyncClient),
		atproto.NewSyncGetCheckoutHandler(pt.SyncClient),
		atproto.NewSyncGetHeadHandler(pt.SyncClient),
		atproto.NewSyncGetLatestCommitHandler(pt.SyncClient),
		atproto.NewSyncGetRecordHandler(pt.SyncClient),
		atproto.NewSyncGetRepoHandler(pt.SyncClient),
		atproto.NewSyncGetRepoStatusHandler(pt.SyncClient),
		atproto.NewSyncListBlobsHandler(pt.SyncClient),
		atproto.NewSyncListReposHandler(pt.SyncClient),
		atproto.NewSyncNotifyOfUpdateHandler(pt.SyncClient),
		atproto.NewSyncRequestCrawlHandler(pt.SyncClient),
		atproto.NewSyncSubscribeReposHandler(pt.SyncClient),
		atproto.NewTempAddReservedHandleHandler(pt.TempClient),
		atproto.NewTempCheckSignupQueueHandler(pt.TempClient),
		atproto.NewTempFetchLabelsHandler(pt.TempClient),
		atproto.NewTempRequestPhoneVerificationHandler(pt.TempClient),
		bsky.NewActorGetPreferencesHandler(pt.ActorClient),
		bsky.NewActorGetProfileHandler(pt.ActorClient),
		bsky.NewActorGetProfilesHandler(pt.ActorClient),
		bsky.NewActorGetSuggestionsHandler(pt.ActorClient),
		bsky.NewActorPutPreferencesHandler(pt.ActorClient),
		bsky.NewActorSearchActorsTypeaheadHandler(pt.ActorClient),
		bsky.NewFeedDescribeFeedGeneratorHandler(pt.FeedClient),
		bsky.NewFeedGetActorFeedsHandler(pt.FeedClient),
		bsky.NewFeedGetActorLikesHandler(pt.FeedClient),
		bsky.NewFeedGetAuthorFeedHandler(pt.FeedClient),
		bsky.NewFeedGetFeedGeneratorHandler(pt.FeedClient),
		bsky.NewFeedGetFeedGeneratorsHandler(pt.FeedClient),
		bsky.NewFeedGetFeedHandler(pt.FeedClient),
		bsky.NewFeedGetFeedSkeletonHandler(pt.FeedClient),
		bsky.NewFeedGetLikesHandler(pt.FeedClient),
		bsky.NewFeedGetListFeedHandler(pt.FeedClient),
		bsky.NewFeedGetPostThreadHandler(pt.FeedClient),
		bsky.NewFeedGetPostsHandler(pt.FeedClient),
		bsky.NewFeedGetQuotesHandler(pt.FeedClient),
		bsky.NewFeedGetRepostedByHandler(pt.FeedClient),
		bsky.NewFeedGetSuggestedFeedsHandler(pt.FeedClient),
		bsky.NewFeedGetTimelineHandler(pt.FeedClient),
		bsky.NewFeedSearchPostsHandler(pt.FeedClient),
		bsky.NewFeedSendInteractionsHandler(pt.FeedClient),
		bsky.NewGraphGetActorStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphGetBlocksHandler(pt.GraphClient),
		bsky.NewGraphGetFollowersHandler(pt.GraphClient),
		bsky.NewGraphGetFollowsHandler(pt.GraphClient),
		bsky.NewGraphGetKnownFollowersHandler(pt.GraphClient),
		bsky.NewGraphGetListBlocksHandler(pt.GraphClient),
		bsky.NewGraphGetListHandler(pt.GraphClient),
		bsky.NewGraphGetListMutesHandler(pt.GraphClient),
		bsky.NewGraphGetListsHandler(pt.GraphClient),
		bsky.NewGraphGetMutesHandler(pt.GraphClient),
		bsky.NewGraphGetStarterPackHandler(pt.GraphClient),
		bsky.NewGraphGetStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphGetSuggestedFollowsByActorHandler(pt.GraphClient),
		bsky.NewGraphMuteActorHandler(pt.GraphClient),
		bsky.NewGraphMuteActorListHandler(pt.GraphClient),
		bsky.NewGraphMuteThreadHandler(pt.GraphClient),
		bsky.NewGraphSearchStarterPacksHandler(pt.GraphClient),
		bsky.NewGraphUnmuteActorHandler(pt.GraphClient),
		bsky.NewGraphUnmuteActorListHandler(pt.GraphClient),
		bsky.NewGraphUnmuteThreadHandler(pt.GraphClient),
		bsky.NewLabelerGetServicesHandler(pt.LabelerClient),
		bsky.NewNotificationGetUnreadCountHandler(pt.NotificationClient),
		bsky.NewNotificationListNotificationsHandler(pt.NotificationClient),
		bsky.NewNotificationPutPreferencesHandler(pt.NotificationClient),
		bsky.NewNotificationRegisterPushHandler(pt.NotificationClient),
		bsky.NewNotificationUpdateSeenHandler(pt.NotificationClient),
		bsky.NewUnspeccedGetConfigHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetPopularFeedGeneratorsHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetSuggestionsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedGetTaggedSuggestionsHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchActorsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchPostsSkeletonHandler(pt.UnspeccedClient),
		bsky.NewUnspeccedSearchStarterPacksSkeletonHandler(pt.UnspeccedClient),
		bsky.NewVideoGetJobStatusHandler(pt.VideoClient),
		bsky.NewVideoGetUploadLimitsHandler(pt.VideoClient),
		bsky.NewVideoUploadVideoHandler(pt.VideoClient),
	)
}

type OzonePassthrough struct {
	*ozone.CommunicationClient
	*ozone.ModerationClient
	*ozone.ServerClient
	*ozone.SetClient
	*ozone.SettingClient
	*ozone.SignatureClient
	*ozone.TeamClient
}

type ChatBskyPassthrough struct {
	*chatbsky.ActorClient
	*chatbsky.ConvoClient
	*chatbsky.ModerationClient
}
