plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.hulaun.leisuretyping"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.hulaun.leisuretyping"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // Signed with the debug key so the APK installs by sideload
            // without a keystore to manage. This is a personal build.
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    sourceSets {
        getByName("test") {
            // One copy of the conformance fixture and golden serves both the Go
            // wrapper and this port. Neither is the reference for the other:
            // both verify against the same file, so a drift in either is caught.
            resources.srcDir("../../testdata/conformance")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // Nothing ships in the APK. The Kotlin stdlib comes with the plugin.
    testImplementation("junit:junit:4.13.2")
}
