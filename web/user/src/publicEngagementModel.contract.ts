import { publicEngagementScore, publicEngagementStats } from './publicEngagementModel'

const imageWithLegacyComments = {
  like_count: 2,
  favorite_count: 3,
  comment_count: 99,
}

const labels = publicEngagementStats(imageWithLegacyComments).map((item) => item.label).join(',')

if (labels !== '点赞,收藏') {
  throw new Error(`public engagement stats must not expose comments, got ${labels}`)
}

const score = publicEngagementScore(imageWithLegacyComments)

if (score !== 13) {
  throw new Error(`public engagement score should ignore legacy comment_count, got ${score}`)
}
