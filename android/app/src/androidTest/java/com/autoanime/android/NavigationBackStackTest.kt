package com.autoanime.android

import androidx.navigation.NavController
import androidx.navigation.testing.TestNavHostController
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NavigationBackStackTest {
    @Test fun logoutActionClearsProtectedDestinationsFromBackStack() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val nav = TestNavHostController(context)
        nav.setGraph(R.navigation.nav_graph)

        nav.navigate(R.id.action_connect_to_library)
        assertEquals(R.id.libraryFragment, nav.currentDestination?.id)
        nav.navigate(R.id.action_library_to_detail)
        assertEquals(R.id.detailFragment, nav.currentDestination?.id)

        nav.navigate(R.id.action_detail_logout)
        assertEquals(R.id.connectFragment, nav.currentDestination?.id)
        assertFalse("logout must remove the prior protected stack", nav.popBackStack())
        assertEquals(R.id.connectFragment, nav.currentDestination?.id)
    }
}
