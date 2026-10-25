package app.pwhs.blockads.service.vpn

import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.io.File

object TailscaleConfigJson {
    class Input(
        val enabled: Boolean,
        val stateDir: String,
        val hostname: String,
        val authKey: String,
        val allowedCIDRs: String,
        val controlURL: String,
    ) {
        // Never let the auth key leak through logs or failure messages.
        override fun toString(): String =
            "Input(enabled=$enabled, stateDir=$stateDir, hostname=$hostname, " +
                "allowedCIDRs=$allowedCIDRs, controlURL=$controlURL)"
    }

    @Serializable
    private data class Payload(
        val stateDir: String,
        val hostname: String,
        val authKey: String,
        val allowedCIDRs: List<String>,
        val controlURL: String? = null,
    )

    private val json = Json {
        encodeDefaults = true
        explicitNulls = false
        ignoreUnknownKeys = true
    }

    fun build(input: Input): String {
        if (!input.enabled) return ""

        val payload = Payload(
            stateDir = input.stateDir,
            hostname = input.hostname,
            authKey = input.authKey,
            allowedCIDRs = input.allowedCIDRs.toCIDRList(),
            controlURL = input.controlURL.trim().takeIf(String::isNotEmpty),
        )
        return json.encodeToString(payload)
    }

    fun stateDir(filesDir: File): String = File(filesDir, "tailscale").absolutePath

    fun hostname(configJson: String): String =
        runCatching { json.decodeFromString<Payload>(configJson).hostname }
            .getOrDefault("blockads-android")

    private fun String.toCIDRList(): List<String> =
        split(',')
            .map(String::trim)
            .filter(String::isNotEmpty)
}
