package com.example.tako;

import org.junit.Test;
import static org.junit.Assert.*;

/**
 * Test class for LibA.
 */
public class LibATest {
    
    private final LibA libA = new LibA();
    
    @Test
    public void testGetVersion() {
        assertEquals("1.0.0", LibA.getVersion());
    }
    
    @Test
    public void testGetLibAMessage() {
        String message = libA.getLibAMessage();
        assertTrue(message.contains("LibA v1.0.0"));
        assertTrue(message.contains("Hello from CoreLib"));
    }
    
    @Test
    public void testEnhancedCalculation() {
        // LibA should add 10 to CoreLib's result (input * 2)
        assertEquals(20, libA.enhancedCalculation(5)); // (5 * 2) + 10 = 20
        assertEquals(10, libA.enhancedCalculation(0)); // (0 * 2) + 10 = 10
        assertEquals(6, libA.enhancedCalculation(-2)); // (-2 * 2) + 10 = 6
    }
    
    @Test
    public void testGetCoreLibVersion() {
        assertEquals("1.0.0", libA.getCoreLibVersion());
    }
}