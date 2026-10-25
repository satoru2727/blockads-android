package app.pwhs.blockads.service.vpn

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class TailscaleConfigJsonTest {

    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `disabled configuration is empty`() {
        val config = TailscaleConfigJson.build(input(enabled = false))

        assertEquals("", config)
    }

    @Test
    fun `build serializes trimmed CIDR list for JSON`() {
        val config = TailscaleConfigJson.build(
            input(allowedCIDRs = "10.0.0.0/8,  fd00::/8,,")
        )
        val parsed = json.parseToJsonElement(config).jsonObject
        val cidrs = parsed.getValue("allowedCIDRs").jsonArray.map { it.jsonPrimitive.content }

        assertEquals(listOf("10.0.0.0/8", "fd00::/8"), cidrs)
    }

    @Test
    fun `build omits a blank control URL`() {
        val parsed = json.parseToJsonElement(TailscaleConfigJson.build(input())).jsonObject

        assertNull(parsed["controlURL"])
    }

    @Test
    fun `build keeps a custom control URL`() {
        val parsed = json
            .parseToJsonElement(TailscaleConfigJson.build(input(controlURL = " https://headscale.example.org ")))
            .jsonObject

        assertEquals("https://headscale.example.org", parsed.getValue("controlURL").jsonPrimitive.content)
    }

    @Test
    fun `state directory is rooted below app files directory`() {
        assertEquals(
            "/data/user/0/app/files/tailscale",
            TailscaleConfigJson.stateDir(File("/data/user/0/app/files"))
        )
    }

    @Test
    fun `auth key is not present in input toString`() {
        val input = input(authKey = "tskey-secret")

        assertFalse(input.toString().contains("tskey-secret"))
        assertTrue(TailscaleConfigJson.build(input).contains("tskey-secret"))
    }

    @Test
    fun `hostname falls back when the payload is unreadable`() {
        assertEquals("blockads-android", TailscaleConfigJson.hostname("not-json"))
        assertEquals("blockads-android", TailscaleConfigJson.hostname(""))
        assertEquals(
            "vibe-phone",
            TailscaleConfigJson.hostname(TailscaleConfigJson.build(input(hostname = "vibe-phone")))
        )
    }

    private fun input(
        enabled: Boolean = true,
        stateDir: String = "/data/user/0/app/files/tailscale",
        hostname: String = "blockads-android",
        authKey: String = "tskey-test",
        allowedCIDRs: String = "10.0.0.0/8",
        controlURL: String = "",
    ) = TailscaleConfigJson.Input(
        enabled = enabled,
        stateDir = stateDir,
        hostname = hostname,
        authKey = authKey,
        allowedCIDRs = allowedCIDRs,
        controlURL = controlURL
    )
}
