include $(TOPDIR)/rules.mk

PKG_NAME:=networkQuality
PKG_VERSION:=0.1
PKG_RELEASE:=0.1
PKG_MAINTAINER:=Will Hawkins (whh8b@obs.cr)
PKG_LICENSE:=GPL-2.0-only

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/network-quality/goresponsiveness.git
#We can configure this once we have a hash that we want to be the source
#of the first release. See
#https://openwrt.org/docs/guide-developer/packages#use_source_repository
#for information on how to use it.
PKG_MIRROR_HASH:=skip
#Note: You will want to make sure that this hash is the one you want!
#Temporarily set the package to build from the HEAD of main.
PKG_SOURCE_VERSION:=af7dd5fa1e683be9bc25dce18c339ed07eaef215

PKG_BUILD_DEPENDS=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0

GO_PKG:=github.com/network-quality/goresponsiveness
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include ../../lang/golang/golang-package.mk

define Package/networkQuality
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Responsiveness Under Working Conditions client
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/networkQuality/description
  Official open-source client for the Responsiveness Under Working Conditions IETF spec.
endef

define Package/networkQuality/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))

  #Because we cannot control the `go build` invocation to get a
  #executable name that we want, let's do this little workaround
  #to make sure that our binary is named `networkQuality`!
	$(INSTALL_DIR) $(1)/usr/bin/
	$(CP) $(PKG_INSTALL_DIR)/usr/bin/goresponsiveness $(1)/usr/bin/networkQuality
endef

$(eval $(call GoBinPackage,networkQuality))
$(eval $(call BuildPackage,networkQuality))
