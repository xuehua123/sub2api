import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import accountPages from './accountPages'
import userBusiness from './userBusiness'
import upstreamWorkspace from './upstreamWorkspace'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
  ...accountPages,
  ...userBusiness,
  ...upstreamWorkspace,
}
