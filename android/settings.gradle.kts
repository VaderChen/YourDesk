pluginManagement { repositories { google(); mavenCentral(); gradlePluginPortal() } }
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        exclusiveContent {
            forRepository { maven { url = uri("libs/maven"); metadataSources { mavenPom(); artifact() } } }
            filter { includeGroup("com.yourdesk.thirdparty") }
        }
        google()
        mavenCentral()
    }
}
rootProject.name = "YourDeskAndroid"
include(":app")
